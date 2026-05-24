package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"relay-ai/internal/config"
)

const (
	cliClaude = "claude"
	cliCodex  = "codex"
)

func newRouter(store *config.Store, logger *Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/anthropic/", makeHandler(store, logger, cliClaude, "/anthropic"))
	mux.HandleFunc("/openai/", makeHandler(store, logger, cliCodex, "/openai"))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return mux
}

func makeHandler(store *config.Store, logger *Logger, cliType, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		allProviders := store.GetEnabledProviders()

		// Filter providers that support this CLI type
		var providers []config.Provider
		for _, p := range allProviders {
			if len(p.CLITypes) == 0 || contains(p.CLITypes, cliType) {
				providers = append(providers, p)
			}
		}

		if len(providers) == 0 {
			logger.Add(RequestLog{
				Method:     r.Method,
				Path:       r.URL.Path,
				CLIType:    cliType,
				StatusCode: http.StatusBadGateway,
				Duration:   time.Since(start).Milliseconds(),
				Error:      "no provider configured for " + cliType,
			})
			http.Error(w, `{"error":{"message":"no provider configured for `+cliType+`"}}`, http.StatusBadGateway)
			return
		}

		// Strip prefix: /anthropic/v1/messages -> /v1/messages
		upstreamPath := strings.TrimPrefix(r.URL.Path, prefix)
		if upstreamPath == "" || upstreamPath == r.URL.Path {
			upstreamPath = r.URL.Path
		}

		// Buffer request body for model transformation and retry
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				logger.Add(RequestLog{
					Method:     r.Method,
					Path:       r.URL.Path,
					CLIType:    cliType,
					StatusCode: http.StatusBadRequest,
					Duration:   time.Since(start).Milliseconds(),
					Error:      "failed to read request body",
				})
				http.Error(w, `{"error":{"message":"failed to read request body"}}`, http.StatusBadRequest)
				return
			}
			r.Body.Close()
		}

		// Extract original model from body
		originalModel := extractModel(bodyBytes)

		// Try each provider in order
		var lastErr string
		for i, provider := range providers {
			target, err := url.Parse(provider.BaseURL)
			if err != nil {
				log.Printf("proxy %s: invalid base_url %q: %v", cliType, provider.BaseURL, err)
				lastErr = fmt.Sprintf("invalid base_url: %v", err)
				continue
			}

			// Apply model transformation per provider
			providerBody := transformBody(bodyBytes, &provider)

			upstreamURL := singleJoiningSlash(target.String(), upstreamPath)
			if r.URL.RawQuery != "" {
				upstreamURL += "?" + r.URL.RawQuery
			}

			log.Printf("proxy %s [%d/%d] %s -> %s", cliType, i+1, len(providers), r.Method, upstreamURL)

			result := tryProvider(w, r, upstreamURL, providerBody, &provider, i < len(providers)-1)
			if result.StatusCode > 0 {
				logger.Add(RequestLog{
					Method:       r.Method,
					Path:         r.URL.Path,
					CLIType:      cliType,
					Provider:     provider.Name,
					Model:        originalModel,
					StatusCode:   result.StatusCode,
					Duration:     time.Since(start).Milliseconds(),
					Error:        result.Error,
					ResponseBody: result.ResponseBody,
				})
				return
			}

			lastErr = result.Error
			log.Printf("proxy %s: provider %q failed, trying next", cliType, provider.Name)
		}

		logger.Add(RequestLog{
			Method:     r.Method,
			Path:       r.URL.Path,
			CLIType:    cliType,
			StatusCode: http.StatusBadGateway,
			Duration:   time.Since(start).Milliseconds(),
			Error:      lastErr,
		})
		http.Error(w, `{"error":{"message":"all providers failed for `+cliType+`"}}`, http.StatusBadGateway)
	}
}

// transformBody applies model name transformation to the request body.
// If DefaultModel is set, all model fields are replaced with it.
// Otherwise, ModelMappings are checked for a matching From value.
func transformBody(body []byte, provider *config.Provider) []byte {
	if len(body) == 0 {
		return body
	}

	if provider.DefaultModel == "" && len(provider.ModelMappings) == 0 {
		return body
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body // not JSON, pass through
	}

	currentModel, _ := m["model"].(string)
	if currentModel == "" {
		return body
	}

	newModel := ""
	if provider.DefaultModel != "" {
		newModel = provider.DefaultModel
	} else {
		for _, mapping := range provider.ModelMappings {
			if mapping.From == currentModel {
				newModel = mapping.To
				break
			}
		}
	}

	if newModel == "" || newModel == currentModel {
		return body
	}

	log.Printf("proxy: model transform %q -> %q (provider: %s)", currentModel, newModel, provider.Name)
	m["model"] = newModel

	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// extractModel reads the "model" field from a JSON body without full parsing.
func extractModel(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	model, _ := m["model"].(string)
	return model
}

// tryProviderResult holds the result of a provider attempt.
type tryProviderResult struct {
	StatusCode   int
	Error        string
	ResponseBody string
}

// tryProvider sends the request to the upstream provider.
func tryProvider(w http.ResponseWriter, r *http.Request, upstreamURL string, body []byte, provider *config.Provider, canFallback bool) tryProviderResult {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, reqBody)
	if err != nil {
		log.Printf("proxy: failed to create request: %v", err)
		return tryProviderResult{Error: fmt.Sprintf("failed to create request: %v", err)}
	}

	req.Header = r.Header.Clone()
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	req.Header.Set("x-api-key", provider.APIKey)

	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("proxy: upstream error from %s: %v", provider.Name, err)
		return tryProviderResult{Error: fmt.Sprintf("upstream error: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if canFallback && resp.StatusCode >= 500 {
		log.Printf("proxy: provider %q returned %d, trying next", provider.Name, resp.StatusCode)
		return tryProviderResult{
			StatusCode:   resp.StatusCode,
			Error:        fmt.Sprintf("upstream returned %d", resp.StatusCode),
			ResponseBody: string(respBody),
		}
	}

	maps.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	errStr := ""
	var respBodyStr string
	if resp.StatusCode >= 400 {
		errStr = fmt.Sprintf("upstream returned %d", resp.StatusCode)
		respBodyStr = string(respBody)
	}
	return tryProviderResult{StatusCode: resp.StatusCode, Error: errStr, ResponseBody: respBodyStr}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
