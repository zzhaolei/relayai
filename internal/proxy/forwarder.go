package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"relay-ai/internal/config"
)

type tryProviderResult struct {
	StatusCode       int
	Error            string
	ResponseBody     string
	ResponseModel    string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	FinishReason     string
}

// UnsupportedContentError indicates the request contains content types
// not supported by the target provider.
type UnsupportedContentError struct {
	Details string
}

func (e *UnsupportedContentError) Error() string {
	return fmt.Sprintf("unsupported content: %s", e.Details)
}

// validateContentForProvider checks if the request body contains content
// that is not supported by the provider (e.g., images for providers that don't support them).
// Returns nil if content is compatible, or UnsupportedContentError if not.
func validateContentForProvider(body []byte, provider *config.Provider) *UnsupportedContentError {
	if len(body) == 0 {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}

	messages, ok := m["messages"].([]any)
	if !ok {
		return nil
	}

	var unsupported []string

	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}

		content, ok := msgMap["content"]
		if !ok {
			continue
		}

		// Check content array for unsupported types
		if contentArr, ok := content.([]any); ok {
			for _, item := range contentArr {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				itemType, _ := itemMap["type"].(string)
				switch itemType {
				case "input_image", "image_url":
					unsupported = append(unsupported, "image (input_image/image_url)")
				case "input_file":
					unsupported = append(unsupported, "file (input_file)")
				}
			}
		}
	}

	// Check tools for unsupported types
	if tools, ok := m["tools"].([]any); ok {
		for _, tool := range tools {
			toolMap, ok := tool.(map[string]any)
			if !ok {
				continue
			}
			toolType, _ := toolMap["type"].(string)
			if toolType != "" && toolType != "function" {
				unsupported = append(unsupported, fmt.Sprintf("tool type '%s'", toolType))
			}
		}
	}

	if len(unsupported) > 0 {
		// Deduplicate
		seen := make(map[string]bool)
		var unique []string
		for _, u := range unsupported {
			if !seen[u] {
				seen[u] = true
				unique = append(unique, u)
			}
		}
		return &UnsupportedContentError{
			Details: strings.Join(unique, ", "),
		}
	}

	return nil
}

func tryProvider(w http.ResponseWriter, r *http.Request, upstreamURL string, body []byte, provider *config.Provider, canFallback bool, convertToResponses bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any) tryProviderResult {
	// Validate content compatibility before making the request
	if !convertToResponses {
		if unsupported := validateContentForProvider(body, provider); unsupported != nil {
			slog.Warn("unsupported content detected",
				"provider", provider.Name,
				"cli_types", provider.CLITypes,
				"details", unsupported.Details,
			)
			return tryProviderResult{
				StatusCode: http.StatusBadRequest,
				Error:      unsupported.Error(),
				ResponseBody: fmt.Sprintf(`{"error":{"type":"unsupported_content_error","message":"Provider '%s' does not support: %s. Please use a provider that supports these content types or remove them from your request.","details":"%s"}}`,
					provider.Name, unsupported.Details, unsupported.Details),
			}
		}
	}

	var writeMu sync.Mutex
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, reqBody)
	if err != nil {
		return tryProviderResult{Error: fmt.Sprintf("failed to create request: %v", err)}
	}

	req.Header = r.Header.Clone()
	req.Header.Del("Accept-Encoding") // Let Go's HTTP client handle decompression automatically
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	isStream := isStreamingRequest(body)
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// For streaming chat-compat: send response.created + start keep-alive BEFORE upstream request.
	// Matches codex-relay: Sse::new(event_stream).keep_alive(KeepAlive::default()) wraps the entire stream
	// including the upstream request wait time.
	var preResponseID string
	var msgItemID string
	var keepAliveDone chan struct{}
	if isStream && convertToResponses {
		preResponseID = sessions.NewID()
		msgItemID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
		setSSEHeaders(w)
		w.WriteHeader(http.StatusOK)
		sendResponseCreated(w, preResponseID, requestModel, &writeMu)

		// Start keep-alive immediately after response.created (matching axum KeepAlive::default() = 15s)
		keepAliveDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-keepAliveDone:
					return
				case <-ticker.C:
					writeMu.Lock()
					if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
						writeMu.Unlock()
						slog.Debug("keep-alive write error (client disconnected)", "error", err)
						return
					}
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					writeMu.Unlock()
				}
			}
		}()
	}

	// Use optimized Transport settings with explicit connection parameters for long SSE streams.
	// http.Client.Timeout stays 0 (no timeout); request context controls lifecycle.
	client := &http.Client{
		Transport: sharedUpstreamTransport,
	}

	// Initial connection retry: only retry on transient network errors (connection refused, reset, TLS timeout,
	// etc.); do not retry on client disconnect (context cancellation).
	var upResp *http.Response
	for attempt := 0; attempt <= upstreamMaxRetries; attempt++ {
		// Rebuild request body on each retry (previous client.Do may have partially consumed it)
		if attempt > 0 {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
			delay := upstreamRetryBaseDelay * (1 << (attempt - 1))
			select {
			case <-r.Context().Done():
				if keepAliveDone != nil {
					close(keepAliveDone)
				}
				return tryProviderResult{Error: fmt.Sprintf("client disconnected during retry: %v", r.Context().Err())}
			case <-time.After(delay):
			}
		}
		upResp, err = client.Do(req)
		if err == nil {
			break
		}
		if !isRetryableNetError(err) || attempt == upstreamMaxRetries {
			if keepAliveDone != nil {
				close(keepAliveDone)
			}
			return tryProviderResult{Error: fmt.Sprintf("upstream error after %d attempt(s): %v", attempt+1, err)}
		}
		slog.Warn("upstream connection failed, retryable", "attempt", attempt+1, "max_retries", upstreamMaxRetries+1, "error", err)
	}
	defer upResp.Body.Close()

	if isStream {
		p, c, t, cached, model, finishReason, accText := forwardStream(r.Context(), w, upResp, convertToResponses, sessions, requestModel, requestMessages, preResponseID, keepAliveDone, &writeMu, false, msgItemID)

		// Auto-continue loop: when finish_reason == "length" in chat-compat mode,
		// the model was truncated by max_output_tokens. Build a continuation request
		// with the accumulated assistant text appended and re-call upstream.
		keepAliveClosed := false
		for continueAttempt := 0; continueAttempt < maxContinueAttempts && finishReason == "length" && convertToResponses; continueAttempt++ {
			// Check if client disconnected before each continuation
			if r.Context().Err() != nil {
				slog.Info("auto-continue: client disconnected, stopping", "attempt", continueAttempt)
				break
			}
			slog.Info("auto-continue triggered",
				"attempt", continueAttempt+1,
				"max", maxContinueAttempts,
				"model", model,
				"accumulated_len", len(accText),
			)

			// Build continuation body: original Chat Completions messages + assistant reply
			continueBody, cbErr := buildContinueBody(body, accText)
			if cbErr != nil {
				slog.Error("auto-continue: failed to build continuation body", "error", cbErr)
				break
			}

			// Make new upstream request with the continuation body
			contReq, crErr := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(continueBody))
			if crErr != nil {
				slog.Error("auto-continue: failed to create request", "error", crErr)
				break
			}
			contReq.Header = req.Header.Clone()
			contReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
			contReq.Header.Set("Accept", "text/event-stream")

			var contResp *http.Response
			for attempt := 0; attempt <= upstreamMaxRetries; attempt++ {
				if attempt > 0 {
					contReq.Body = io.NopCloser(bytes.NewReader(continueBody))
					contReq.ContentLength = int64(len(continueBody))
					delay := upstreamRetryBaseDelay * (1 << (attempt - 1))
					select {
					case <-r.Context().Done():
						break
					case <-time.After(delay):
					}
				}
				contResp, crErr = client.Do(contReq)
				if crErr == nil {
					break
				}
				if !isRetryableNetError(crErr) || attempt == upstreamMaxRetries {
					slog.Error("auto-continue: upstream request failed", "attempt", attempt+1, "error", crErr)
					break
				}
			}
			if crErr != nil || contResp == nil {
				break
			}

			// Continue streaming: isContinuation=true skips response.created and output_item.added
			cp, cc, ct, cch, _, cReason, cText := forwardStream(r.Context(), w, contResp, convertToResponses, sessions, model, requestMessages, preResponseID, keepAliveDone, &writeMu, true, msgItemID)
			contResp.Body.Close()
			p += cp
			c += cc
			t += ct
			cached += cch
			accText += cText
			finishReason = cReason
		}

		// After continuation loop ends (or was skipped), emit final completion events.
		// forwardStream with isContinuation=true skipped output_item.done and response.completed,
		// so we emit them here with the full accumulated text and final usage.
		if convertToResponses && r.Context().Err() == nil {
			emitFinalCompletionEvents(w, preResponseID, model, sessions, accText, p, c, t, finishReason, &writeMu, msgItemID)
		}

		if keepAliveDone != nil && !keepAliveClosed {
			close(keepAliveDone)
			keepAliveClosed = true
		}
		return tryProviderResult{StatusCode: upResp.StatusCode, ResponseModel: model, PromptTokens: p, CompletionTokens: c, TotalTokens: t, CachedTokens: cached, FinishReason: finishReason}
	}

	respBody, readErr := io.ReadAll(upResp.Body)
	if readErr != nil {
		slog.Error("upstream body read error", "status", upResp.StatusCode, "model", requestModel, "error", readErr)
	}

	// Log HTTP error responses for debugging
	if upResp.StatusCode >= 400 {
		slog.Warn("upstream HTTP error",
			"provider", provider.Name,
			"model", requestModel,
			"status", upResp.StatusCode,
			"response", sanitizeResponseBody(respBody),
		)
	}

	// Fallback on server errors (5xx) and certain client errors (400 Bad Request, 415 Unsupported Media Type)
	// These often indicate content format issues that another provider might handle better
	if canFallback && (upResp.StatusCode >= 500 || upResp.StatusCode == 400 || upResp.StatusCode == 415) {
		slog.Warn("triggering fallback due to HTTP error",
			"provider", provider.Name,
			"status", upResp.StatusCode,
		)
		return tryProviderResult{ResponseBody: sanitizeResponseBody(respBody)}
	}

	if convertToResponses {
		if requestModel == "" {
			requestModel = extractModel(body)
		}
		responseID := preResponseID
		if responseID == "" {
			responseID = sessions.NewID()
		}
		respBody, _ = fromChatResponse(respBody, responseID, requestModel, sessions)
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(upResp.StatusCode)
	if _, err := w.Write(respBody); err != nil {
		slog.Error("response write error", "model", requestModel, "error", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	var respBodyStr string
	if upResp.StatusCode >= 400 {
		respBodyStr = sanitizeResponseBody(respBody)
	}
	p, c, t, cached := extractTokenUsage(respBody)
	responseModel := extractResponseModel(respBody)

	return tryProviderResult{StatusCode: upResp.StatusCode, ResponseBody: respBodyStr, ResponseModel: responseModel, PromptTokens: p, CompletionTokens: c, TotalTokens: t, CachedTokens: cached}
}

// extractResponseModel extracts the model field from a response body.
func extractResponseModel(body []byte) string {
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

// forwardStream forwards the upstream SSE response, optionally converting to Responses API format.
func forwardStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, convert bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any, preResponseID string, keepAliveDone chan struct{}, writeMu *sync.Mutex, isContinuation bool, msgItemID string) (promptTokens, completionTokens, totalTokens int, cachedTokens int, responseModel string, finishReason string, accumulatedText string) {
	flusher, canFlush := w.(http.Flusher)

	if convert {
		ct := resp.Header.Get("Content-Type")
		if resp.StatusCode != 200 || !strings.Contains(ct, "text/event-stream") {
			respBody, _ := io.ReadAll(resp.Body)
			// Try to convert error response
			model := requestModel
			if model == "" {
				model = "unknown"
			}
			responseID := preResponseID
			if responseID == "" {
				responseID = sessions.NewID()
			}
			convertedBody, _ := fromChatResponse(respBody, responseID, model, sessions)
			p, c, t := synthesizeResponsesSSE(w, convertedBody, flusher, canFlush, model, sessions, preResponseID)
			return p, c, t, 0, "", "", ""
		}
		// For streaming mode
		model := requestModel
		if model == "" {
			model = "unknown"
		}
		p, c, t, cached, reason, text := translateStream(ctx, w, resp, flusher, canFlush, model, sessions, requestMessages, preResponseID, keepAliveDone, writeMu, isContinuation, msgItemID)
		return p, c, t, cached, "", reason, text
	}

	// Passthrough mode
	setSSEHeaders(w)
	if v := resp.Header.Get("x-request-id"); v != "" {
		w.Header().Set("x-request-id", v)
	}
	w.WriteHeader(resp.StatusCode)

	// Start keep-alive to prevent client disconnect during long thinking phases
	var passthroughKeepAliveDone chan struct{}
	if canFlush {
		passthroughKeepAliveDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-passthroughKeepAliveDone:
					return
				case <-ticker.C:
					writeMu.Lock()
					if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
						writeMu.Unlock()
						return
					}
					flusher.Flush()
					writeMu.Unlock()
				}
			}
		}()
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(nil, 500*1024*1024)

	// Upstream read timeout detection for passthrough mode
	fwdStart := time.Now()
	var fwdLastActivityUnixNano int64 = time.Now().UnixNano()
	fwdStopMonitor := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-fwdStopMonitor:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				last := time.Unix(0, fwdLastActivityUnixNano)
				silence := time.Since(last)
				if silence > upstreamReadTimeout {
					slog.Warn("upstream passthrough read timeout, closing connection", "silence", silence.String(), "model", requestModel)
					resp.Body.Close()
					return
				}
			}
		}
	}()
	defer close(fwdStopMonitor)

	// Watch for client disconnect
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	// Track token usage and model from Anthropic streaming events
	var streamPromptTokens, streamCompletionTokens, streamCachedTokens int
	var streamModel string
	var currentEventType string

	fwdChunkCount := 0
	for scanner.Scan() {
		fwdLastActivityUnixNano = time.Now().UnixNano()
		fwdChunkCount++
		line := scanner.Text()

		// Parse Anthropic SSE events to extract token usage and model
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			currentEventType = after
		} else if after, ok := strings.CutPrefix(line, "data: "); ok {
			switch currentEventType {
			case "message_start":
				// message_start contains message.model and message.usage
				var event struct {
					Type    string `json:"type"`
					Message struct {
						Model string `json:"model"`
						Usage struct {
							InputTokens         int `json:"input_tokens"`
							OutputTokens        int `json:"output_tokens"`
							CacheReadTokens     int `json:"cache_read_input_tokens"`
							CacheCreationTokens int `json:"cache_creation_input_tokens"`
						} `json:"usage"`
					} `json:"message"`
				}
				dataStr := after
				if json.Unmarshal([]byte(dataStr), &event) == nil {
					if event.Message.Model != "" {
						streamModel = event.Message.Model
					}
					if event.Message.Usage.InputTokens > 0 {
						streamPromptTokens = event.Message.Usage.InputTokens
					}
					if event.Message.Usage.CacheReadTokens > 0 {
						streamCompletionTokens = 0 // reset, will be set by message_delta
						streamCachedTokens = event.Message.Usage.CacheReadTokens
						// Anthropic's input_tokens does NOT include cached tokens;
						// add them so prompt_tokens represents total input (matches OpenAI convention).
						streamPromptTokens += event.Message.Usage.CacheReadTokens
					}
				}
			case "message_delta":
				// message_delta contains usage.output_tokens
				var event struct {
					Type  string `json:"type"`
					Usage struct {
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				}
				dataStr := strings.TrimPrefix(line, "data: ")
				if json.Unmarshal([]byte(dataStr), &event) == nil {
					if event.Usage.OutputTokens > 0 {
						streamCompletionTokens = event.Usage.OutputTokens
					}
				}
			}
		}

		writeMu.Lock()
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			writeMu.Unlock()
			slog.Error("passthrough write error, client disconnected", "model", requestModel, "error", err)
			resp.Body.Close()
			break
		}
		if line == "" {
			if canFlush {
				flusher.Flush()
			}
		}
		writeMu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		errCategory := "unknown"
		switch {
		case errors.Is(err, io.EOF):
			errCategory = "EOF (upstream closed connection)"
		case errors.Is(err, context.Canceled):
			errCategory = "client_disconnected"
		case errors.Is(err, context.DeadlineExceeded):
			errCategory = "context_deadline"
		default:
			var netErr net.Error
			if errors.As(err, &netErr) {
				if netErr.Timeout() {
					errCategory = "network_timeout"
				} else {
					errCategory = "network_error"
				}
			} else {
				errCategory = "connection_reset"
			}
		}
		slog.Error("passthrough scanner error", "category", errCategory, "model", requestModel, "error", err)
	}
	// Log stream completion at appropriate level
	logLevel := slog.LevelDebug
	logMsg := "passthrough stream completed"
	logArgs := []any{
		"chunks", fwdChunkCount,
		"duration", time.Since(fwdStart).String(),
		"request_model", requestModel,
		"response_model", streamModel,
		"prompt_tokens", streamPromptTokens,
		"completion_tokens", streamCompletionTokens,
	}
	if fwdChunkCount == 0 {
		logLevel = slog.LevelWarn
		logMsg = "passthrough stream completed with no data chunks"
	} else if streamPromptTokens == 0 && streamCompletionTokens == 0 {
		logArgs = append(logArgs, "note", "no token usage data extracted")
	}
	slog.Log(ctx, logLevel, logMsg, logArgs...)
	if passthroughKeepAliveDone != nil {
		close(passthroughKeepAliveDone)
	}
	if canFlush {
		flusher.Flush()
	}
	return streamPromptTokens, streamCompletionTokens, streamPromptTokens + streamCompletionTokens, streamCachedTokens, streamModel, "", ""
}

// buildContinueBody constructs a continuation Chat Completions request body.
// It appends the accumulated assistant text to the original messages so the model
// can resume generation from where it was truncated.
func buildContinueBody(originalBody []byte, accumulatedText string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(originalBody, &m); err != nil {
		return nil, fmt.Errorf("failed to parse original body: %w", err)
	}

	// Get existing messages
	messages, ok := m["messages"].([]any)
	if !ok {
		return nil, errors.New("no messages array in body")
	}

	// Append assistant message with the accumulated (truncated) text
	messages = append(messages, map[string]any{
		"role":    "assistant",
		"content": accumulatedText,
	})

	m["messages"] = messages
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal continuation body: %w", err)
	}
	return out, nil
}

// emitFinalCompletionEvents writes the Responses API SSE completion events
// (response.output_item.done + response.completed) after the auto-continue loop.
// This is the counterpart of translateStream's finalization when isContinuation=true
// caused it to skip those events.
func emitFinalCompletionEvents(w http.ResponseWriter, responseID, requestModel string, sessions *SessionStore, accumulatedText string, promptTokens, completionTokens, totalTokens int, finishReason string, writeMu *sync.Mutex, msgItemID string) {

	writeSSEEvent := func(eventType string, data any) {
		jsonData, _ := jsonMarshalSafe(data)
		writeMu.Lock()
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData); err != nil {
			writeMu.Unlock()
			slog.Error("final completion event write error", "event", eventType, "error", err)
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		writeMu.Unlock()
	}

	// response.output_item.done with the full accumulated text
	writeSSEEvent("response.output_item.done", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":   "message",
			"id":     msgItemID,
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": accumulatedText},
			},
		},
	})

	// response.completed
	status := "completed"
	if finishReason == "length" || finishReason == "content_filter" || finishReason == "stream_incomplete" {
		status = "incomplete"
	}
	completedResp := map[string]any{
		"id":     responseID,
		"status": status,
		"model":  requestModel,
		"output": []any{
			map[string]any{
				"type":   "message",
				"id":     msgItemID,
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{"type": "output_text", "text": accumulatedText},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  promptTokens,
			"output_tokens": completionTokens,
			"total_tokens":  totalTokens,
		},
	}
	if status == "incomplete" {
		reason := "max_output_tokens"
		if finishReason == "stream_incomplete" {
			reason = "stream_disconnected"
		}
		completedResp["incomplete_details"] = map[string]any{"reason": reason}
	}
	writeSSEEvent("response.completed", map[string]any{
		"response": completedResp,
	})

	slog.Info("final completion events emitted",
		"model", requestModel,
		"status", status,
		"accumulated_len", len(accumulatedText),
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
	)
}
