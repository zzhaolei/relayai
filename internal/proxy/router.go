package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"relay-ai/internal/config"
)

func jsonMarshalSafe(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	// Encode appends a trailing newline; strip it.
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

const (
	cliClaude = "claude"
	cliCodex  = "codex"
)

func newRouter(store *config.Store, logger *Logger, sessions *SessionStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/anthropic/{path...}", makeHandler(store, logger, sessions, cliClaude, "/anthropic"))
	mux.HandleFunc("/openai/{path...}", makeHandler(store, logger, sessions, cliCodex, "/openai"))
	mux.HandleFunc("/v1/responses", makeHandler(store, logger, sessions, cliCodex, ""))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			slog.Error("health write error", "error", err)
		}
	})

	return mux
}

func makeHandler(store *config.Store, logger *Logger, sessions *SessionStore, cliType, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var providers []config.Provider
		if token := extractBearerToken(r); strings.HasPrefix(token, "sk-local-") {
			// 代理级 token：走默认路由（所有启用的 provider）
			if token == store.GetProxyAuthToken() {
				// fall through to default routing below
			} else if p := store.GetProviderByAuthToken(token); p != nil {
				// 提供商级 token：路由到特定 provider
				providers = []config.Provider{*p}
			}
		}
		if len(providers) == 0 {
			for _, p := range store.GetEnabledProviders() {
				if len(p.CLITypes) == 0 || slices.Contains(p.CLITypes, cliType) {
					providers = append(providers, p)
				}
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

		upstreamPath := strings.TrimPrefix(r.URL.Path, prefix)
		if upstreamPath == "" || upstreamPath == r.URL.Path {
			upstreamPath = r.URL.Path
		}

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

		isResponsesPath := cliType == cliCodex && (strings.HasSuffix(r.URL.Path, "/responses") || strings.HasSuffix(r.URL.Path, "/v1/responses"))
		chatCompatMode := len(providers) > 0 && providers[0].ChatCompatMode
		var requestModel string
		var requestMessages []map[string]any

		if isResponsesPath && chatCompatMode {
			// Chat compat mode: Responses API → Chat Completions
			if before, ok := strings.CutSuffix(upstreamPath, "/v1/responses"); ok {
				upstreamPath = before + "/v1/chat/completions"
			} else if before, ok := strings.CutSuffix(upstreamPath, "/responses"); ok {
				upstreamPath = before + "/chat/completions"
			}
			bodyBytes, requestModel = toChatRequest(bodyBytes, sessions)

			// Extract messages for session history
			var reqMap map[string]any
			if json.Unmarshal(bodyBytes, &reqMap) == nil {
				if msgs, ok := reqMap["messages"].([]any); ok {
					for _, m := range msgs {
						if mm, ok := m.(map[string]any); ok {
							requestMessages = append(requestMessages, mm)
						}
					}
				}
			}
		}

		// Ensure stream_options for streaming requests
		bodyBytes = ensureStreamOptions(bodyBytes)

		var lastErr string
		var lastRespBody string
		var lastUpstreamURL string
		for i, provider := range providers {
			target, err := url.Parse(provider.BaseURL)
			if err != nil {
				slog.Error("invalid provider base_url", "cli", cliType, "base_url", provider.BaseURL, "error", err)
				lastErr = fmt.Sprintf("invalid base_url: %v", err)
				continue
			}

			providerBody := transformBody(bodyBytes, &provider)
			actualModel := extractModel(providerBody)

			upstreamURL := joinURL(target, upstreamPath)
			if r.URL.RawQuery != "" {
				upstreamURL += "?" + r.URL.RawQuery
			}

			needResponseConversion := isResponsesPath && chatCompatMode
			canFallback := i < len(providers)-1
			result := tryProvider(w, r, upstreamURL, providerBody, &provider, canFallback, needResponseConversion, sessions, requestModel, requestMessages)
			if result.StatusCode > 0 {
				logger.Add(RequestLog{
					Method:           r.Method,
					Path:             upstreamURL,
					UpstreamURL:      upstreamURL,
					CLIType:          cliType,
					ProviderID:       provider.ID,
					Provider:         provider.Name,
					Model:            actualModel,
					StatusCode:       result.StatusCode,
					Duration:         time.Since(start).Milliseconds(),
					ResponseBody:     result.ResponseBody,
					PromptTokens:     result.PromptTokens,
					CompletionTokens: result.CompletionTokens,
					TotalTokens:      result.TotalTokens,
				})
				return
			}

			lastErr = result.Error
			lastRespBody = result.ResponseBody
			lastUpstreamURL = upstreamURL
			slog.Warn("provider failed, trying next", "cli", cliType, "provider", provider.Name)
		}

		logger.Add(RequestLog{
			Method:       r.Method,
			Path:         lastUpstreamURL,
			UpstreamURL:  lastUpstreamURL,
			CLIType:      cliType,
			StatusCode:   http.StatusBadGateway,
			Duration:     time.Since(start).Milliseconds(),
			Error:        lastErr,
			ResponseBody: lastRespBody,
		})
		http.Error(w, `{"error":{"message":"all providers failed for `+cliType+`"}}`, http.StatusBadGateway)
	}
}

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

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func joinURL(target *url.URL, upstreamPath string) string {
	basePath := strings.TrimRight(target.Path, "/")
	if basePath != "" && basePath != "/" && strings.HasPrefix(upstreamPath, basePath) {
		upstreamPath = strings.TrimPrefix(upstreamPath, basePath)
		if !strings.HasPrefix(upstreamPath, "/") {
			upstreamPath = "/" + upstreamPath
		}
	}
	return singleJoiningSlash(target.String(), upstreamPath)
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

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		auth = r.Header.Get("x-api-key")
		if auth == "" {
			return ""
		}
		return auth
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return auth
}

func extractTokenUsage(body []byte) (int, int, int) {
	if len(body) == 0 {
		return 0, 0, 0
	}
	var openai struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &openai) == nil && openai.Usage.TotalTokens > 0 {
		return openai.Usage.PromptTokens, openai.Usage.CompletionTokens, openai.Usage.TotalTokens
	}
	var anthropic struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &anthropic) == nil && anthropic.Usage.InputTokens > 0 {
		return anthropic.Usage.InputTokens, anthropic.Usage.OutputTokens, anthropic.Usage.InputTokens + anthropic.Usage.OutputTokens
	}
	return 0, 0, 0
}

func sanitizeResponseBody(body []byte) string {
	const maxLen = 4096
	s := string(body)
	var cleaned strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r >= 32 {
			cleaned.WriteRune(r)
		}
	}
	result := cleaned.String()
	if len(result) > maxLen {
		result = result[:maxLen] + "...(truncated)"
	}
	return result
}
