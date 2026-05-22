package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"one-switch/internal/config"
)

const (
	cliClaude = "claude"
	cliCodex  = "codex"
	cliGemini = "gemini"
)

func newRouter(store *config.Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/anthropic/", makeHandler(store, cliClaude, "/anthropic"))
	mux.HandleFunc("/openai/", makeHandler(store, cliCodex, "/openai"))
	mux.HandleFunc("/gemini/", makeHandler(store, cliGemini, "/gemini"))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return mux
}

func makeHandler(store *config.Store, cliType, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providers := store.GetEnabledProviders()
		if len(providers) == 0 {
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
				http.Error(w, `{"error":{"message":"failed to read request body"}}`, http.StatusBadRequest)
				return
			}
			r.Body.Close()
		}

		// Try each provider in order
		for i, provider := range providers {
			target, err := url.Parse(provider.BaseURL)
			if err != nil {
				log.Printf("proxy %s: invalid base_url %q: %v", cliType, provider.BaseURL, err)
				continue
			}

			// Apply model transformation per provider
			providerBody := transformBody(bodyBytes, &provider)

			upstreamURL := singleJoiningSlash(target.String(), upstreamPath)
			if r.URL.RawQuery != "" {
				upstreamURL += "?" + r.URL.RawQuery
			}

			log.Printf("proxy %s [%d/%d] %s -> %s", cliType, i+1, len(providers), r.Method, upstreamURL)

			ok := tryProvider(w, r, upstreamURL, providerBody, &provider, i < len(providers)-1)
			if ok {
				return
			}

			log.Printf("proxy %s: provider %q failed, trying next", cliType, provider.Name)
		}

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

func tryProvider(w http.ResponseWriter, r *http.Request, upstreamURL string, body []byte, provider *config.Provider, canFallback bool) bool {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, reqBody)
	if err != nil {
		log.Printf("proxy: failed to create request: %v", err)
		return false
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
		return false
	}
	defer resp.Body.Close()

	if canFallback && resp.StatusCode >= 500 {
		log.Printf("proxy: provider %q returned %d, trying next", provider.Name, resp.StatusCode)
		io.Copy(io.Discard, resp.Body)
		return false
	}

	maps.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	return true
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
