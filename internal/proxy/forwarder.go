package proxy

import (
	"bufio"
	"bytes"
	"context"
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
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func tryProvider(w http.ResponseWriter, r *http.Request, upstreamURL string, body []byte, provider *config.Provider, canFallback bool, convertToResponses bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any) tryProviderResult {
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
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	isStream := isStreamingRequest(body)
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// For streaming chat-compat: send response.created + start keep-alive BEFORE upstream request.
	// Matches codex-relay: Sse::new(event_stream).keep_alive(KeepAlive::default()) wraps the entire stream
	// including the upstream request wait time.
	var preResponseID string
	var keepAliveDone chan struct{}
	if isStream && convertToResponses {
		preResponseID = sessions.NewID()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
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

	// 使用优化的 Transport 配置，显式设置连接参数以适配长 SSE 流。
	// http.Client.Timeout 保持为 0（不设超时），由请求 context 控制生命周期。
	client := &http.Client{
		Transport: sharedUpstreamTransport,
	}

	// 初始连接重试：仅对瞬态网络错误（连接拒绝、重置、TLS 超时等）进行重试，
	// 客户端主动断开（context 取消）不重试。
	var upResp *http.Response
	for attempt := 0; attempt <= upstreamMaxRetries; attempt++ {
		// 每次重试需要重建 request body（上一次 client.Do 可能已部分消费）
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
		p, c, t := forwardStream(r.Context(), w, upResp, convertToResponses, sessions, requestModel, requestMessages, preResponseID, keepAliveDone, &writeMu)
		if keepAliveDone != nil {
			close(keepAliveDone)
		}
		return tryProviderResult{StatusCode: upResp.StatusCode, PromptTokens: p, CompletionTokens: c, TotalTokens: t}
	}

	respBody, readErr := io.ReadAll(upResp.Body)
	if readErr != nil {
		slog.Error("upstream body read error", "status", upResp.StatusCode, "model", requestModel, "error", readErr)
	}

	if canFallback && upResp.StatusCode >= 500 {
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
	p, c, t := extractTokenUsage(respBody)

	return tryProviderResult{StatusCode: upResp.StatusCode, ResponseBody: respBodyStr, PromptTokens: p, CompletionTokens: c, TotalTokens: t}
}

// forwardStream forwards the upstream SSE response, optionally converting to Responses API format.
func forwardStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, convert bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any, preResponseID string, keepAliveDone chan struct{}, writeMu *sync.Mutex) (promptTokens, completionTokens, totalTokens int) {
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
			return p, c, t
		}
		// For streaming mode
		model := requestModel
		if model == "" {
			model = "unknown"
		}
		p, c, t := translateStream(ctx, w, resp, flusher, canFlush, model, sessions, requestMessages, preResponseID, keepAliveDone, writeMu)
		return p, c, t
	}

	// Passthrough mode
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		w.Header().Set("x-request-id", v)
	}
	w.WriteHeader(resp.StatusCode)

	// 启动 keep-alive，避免深度思考时客户端因长时间无数据而断开
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
				if silence > 3*time.Minute {
					slog.Warn("upstream passthrough read timeout, closing connection", "silence", silence.String(), "model", requestModel)
					resp.Body.Close()
					return
				}
			}
		}
	}()
	defer close(fwdStopMonitor)

	// 监听客户端断开信号
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	fwdChunkCount := 0
	for scanner.Scan() {
		fwdLastActivityUnixNano = time.Now().UnixNano()
		fwdChunkCount++
		line := scanner.Text()
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
	slog.Debug("passthrough stream completed",
		"chunks", fwdChunkCount,
		"duration", time.Since(fwdStart).String(),
		"model", requestModel,
	)
	if passthroughKeepAliveDone != nil {
		close(passthroughKeepAliveDone)
	}
	if canFlush {
		flusher.Flush()
	}
	return
}
