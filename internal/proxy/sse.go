package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type toolCallAccum struct {
	id        string
	name      string
	arguments strings.Builder
	itemID    string
}

const (
	// upstreamMaxRetries is the max retry count for initial upstream connection failures.
	upstreamMaxRetries = 3
	// upstreamRetryBaseDelay is the base delay for exponential backoff on retry.
	upstreamRetryBaseDelay = 1 * time.Second
)

// sharedUpstreamTransport is a shared http.Transport with optimized settings for SSE streaming.
// Reused across all requests to benefit from connection pooling.
// Key design choices:
//   - Keep-alives enabled to prevent intermediate network devices (NAT, firewalls)
//     from dropping idle TCP connections during long-running SSE streams.
//   - TLS handshake timeout set generously since upstream providers may be slow on first connect.
//   - IdleConnTimeout only affects pooled idle connections, NOT active SSE streams being read from.
var sharedUpstreamTransport = &http.Transport{
	ForceAttemptHTTP2:     true,
	DisableKeepAlives:     false,
	TLSHandshakeTimeout:   30 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second, // Abort if upstream stalls before sending any headers
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
}

func isRetryableNetError(err error) bool {
	if err == nil {
		return false
	}
	// Client disconnected — don't retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// net.Error with Timeout() == true is retryable.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// net.OpError wraps most dial/read/write errors (connection refused, reset, etc.).
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// Fallback: keyword match for edge-case error strings from various Go versions.
	errMsg := err.Error()
	for _, kw := range []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected EOF",
		"server closed idle connection",
		"i/o timeout",
		"TLS handshake",
	} {
		if strings.Contains(errMsg, kw) {
			return true
		}
	}
	return false
}

// translateStream converts an upstream Chat Completions SSE stream into Responses API SSE.
func translateStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher, canFlush bool, requestModel string, sessions *SessionStore, requestMessages []map[string]any, preResponseID string, keepAliveDone chan struct{}, writeMu *sync.Mutex) (promptTokens, completionTokens, totalTokens int) {
	responseID := preResponseID
	if responseID == "" {
		responseID = sessions.NewID()
	}
	// msg_item_id uses an independent id, matching codex-relay (separate UUID)
	msgItemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	// Ensure we have a flusher for SSE streaming
	if flusher == nil {
		flusher, canFlush = w.(http.Flusher)
	}
	// When preResponseID is set, the caller (tryProvider) already sent headers
	// and response.created before invoking translateStream — skip re-sending.
	headersSent := preResponseID != ""

	ensureHeaders := func() {
		if headersSent {
			return
		}
		headersSent = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(resp.StatusCode)
	}

	// writeEvent sends a Responses API SSE event.
	// "type" field is always first, matching codex-relay.
	seq := 0
	writeEvent := func(eventType string, fields map[string]any) {
		ensureHeaders()
		seq++
		fieldsJSON, _ := jsonMarshalSafe(fields)
		b := make([]byte, 0, len(fieldsJSON)+len(eventType)+12)
		b = append(b, `{"type":"`...)
		b = append(b, eventType...)
		b = append(b, `",`...)
		b = append(b, fieldsJSON[1:]...)
		if seq <= 10 || eventType == "response.completed" || eventType == "response.output_item.done" {
		}
		writeMu.Lock()
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b); err != nil {
			writeMu.Unlock()
			slog.Error("codex-sse write error", "events", seq, "model", requestModel, "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}
		writeMu.Unlock()
	}

	// Emit response.created if not already sent before upstream request
	if preResponseID == "" {
		writeEvent("response.created", map[string]any{
			"response": map[string]any{
				"id":     responseID,
				"status": "in_progress",
				"model":  requestModel,
			},
		})
	}

	var (
		accumulatedText      strings.Builder
		accumulatedReasoning strings.Builder
		toolCalls            = make(map[int]*toolCallAccum)
		emittedMessageItem   bool
		streamDone           bool
		streamFinishReason   string
		streamUsage          struct {
			prompt, completion, total int
		}
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(nil, 500*1024*1024)

	streamStart := time.Now()
	// Track last upstream read activity for timeout detection.
	// When the upstream enters a long thinking phase and the connection silently
	// drops, bufio.Scanner.Scan() blocks indefinitely. This goroutine monitors
	// read activity and closes resp.Body after 3 minutes of silence.
	var lastActivityUnixNano int64 = time.Now().UnixNano()
	stopMonitor := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopMonitor:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				last := time.Unix(0, lastActivityUnixNano)
				silence := time.Since(last)
				if silence > 3*time.Minute {
					slog.Warn("upstream read timeout, closing connection", "silence", silence.String(), "model", requestModel)
					resp.Body.Close()
					return
				}
			}
		}
	}()
	defer close(stopMonitor)

	// 监听客户端断开信号
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	chunkCount := 0
	toolCallNames := make([]string, 0)
	for scanner.Scan() {
		lastActivityUnixNano = time.Now().UnixNano()
		line := scanner.Text()
		payload, ok := extractSSEData(line)
		if !ok {
			continue
		}
		if payload == "[DONE]" {
			streamDone = true
			break
		}

		chunkCount++
		if chunkCount <= 3 {
		}

		var chunk struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content          *string `json:"content"`
					ReasoningContent *string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			slog.Error("codex-sse chunk parse error", "chunk", chunkCount, "error", err)
			continue // 跳过这个 chunk，继续处理下一个
		}
		if len(chunk.Choices) == 0 {
			if chunk.Usage != nil {
				streamUsage.prompt = chunk.Usage.PromptTokens
				streamUsage.completion = chunk.Usage.CompletionTokens
				streamUsage.total = chunk.Usage.TotalTokens
			}
			continue
		}
		if chunk.Usage != nil {
			streamUsage.prompt = chunk.Usage.PromptTokens
			streamUsage.completion = chunk.Usage.CompletionTokens
			streamUsage.total = chunk.Usage.TotalTokens
		}

		choice := chunk.Choices[0]

		// Reasoning content — accumulate only (no SSE events, matching codex-relay)
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			accumulatedReasoning.WriteString(*choice.Delta.ReasoningContent)
		}

		// Track finish reason
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			streamFinishReason = *choice.FinishReason
		}

		// Text content delta
		content := choice.Delta.Content
		if content != nil && *content != "" {
			if !emittedMessageItem {
				writeEvent("response.output_item.added", map[string]any{
					"output_index": 0,
					"item": map[string]any{
						"id":      msgItemID,
						"type":    "message",
						"role":    "assistant",
						"status":  "in_progress",
						"content": []any{},
					},
				})
				emittedMessageItem = true
			}
			accumulatedText.WriteString(*content)
			writeEvent("response.output_text.delta", map[string]any{
				"item_id":      msgItemID,
				"output_index": 0,
				"delta":        *content,
			})
		}

		// Tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			stored, exists := toolCalls[idx]
			if !exists {
				callID := tc.ID
				if callID == "" {
					callID = fmt.Sprintf("call_%s_%d", responseID, idx)
				}
				name := tc.Function.Name
				stored = &toolCallAccum{
					id:     callID,
					name:   name,
					itemID: fmt.Sprintf("fc_%s_%d", responseID, idx),
				}
				toolCalls[idx] = stored
			} else {
				if tc.ID != "" {
					stored.id = tc.ID
				}
				if tc.Function.Name != "" {
					stored.name += tc.Function.Name
				}
			}
			if tc.Function.Arguments != "" {
				stored.arguments.WriteString(tc.Function.Arguments)
			}
		}
	}

	// Log stream diagnostics (matching codex-relay debug logging)
	if len(toolCalls) > 0 {
		tcNames := make([]string, 0, len(toolCalls))
		for i := 0; i < len(toolCalls); i++ {
			if tc, ok := toolCalls[i]; ok {
				tcNames = append(tcNames, tc.name)
			}
		}
		toolCallNames = tcNames
	}
	slog.Debug("stream completed",
		"chunks", chunkCount,
		"tool_calls", toolCallNames,
		"duration", time.Since(streamStart).String(),
		"done", streamDone,
		"model", requestModel,
	)

	// Check for scanner errors (upstream connection drop, timeout, etc.)
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
		slog.Error("codex-sse scanner error", "chunks", chunkCount, "done", streamDone, "category", errCategory, "model", requestModel, "error", err)
	}
	// --- Finalize ---
	ensureHeaders()

	if !streamDone {
		// Stream disconnected before [DONE]
		writeEvent("response.failed", map[string]any{
			"response": map[string]any{
				"id":     responseID,
				"status": "failed",
				"error": map[string]any{
					"code":    "stream_incomplete",
					"message": "stream disconnected before completion",
				},
			},
		})
		if canFlush {
			flusher.Flush()
		}
		return
	}

	// Message output_item.done
	if emittedMessageItem {
		writeEvent("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"type":   "message",
				"id":     msgItemID,
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{"type": "output_text", "text": accumulatedText.String()},
				},
			},
		})
	} else if len(toolCalls) == 0 {
		// 纯推理模型无实际 content 时，输出空 message（对齐 codex-relay）
		writeEvent("response.output_item.added", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"id":      msgItemID,
				"type":    "message",
				"role":    "assistant",
				"status":  "in_progress",
				"content": []any{},
			},
		})
		writeEvent("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"type":   "message",
				"id":     msgItemID,
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{"type": "output_text", "text": ""},
				},
			},
		})
		emittedMessageItem = true
	}

	// Function call items
	baseIndex := 1
	if !emittedMessageItem {
		baseIndex = 0
	}
	var fcItems []map[string]any
	for i := 0; i < len(toolCalls); i++ {
		tc, ok := toolCalls[i]
		if !ok {
			continue
		}
		outputIndex := baseIndex + i

		namespace, name := splitMCPName(tc.name)
		args := tc.arguments.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}

		addedItem := map[string]any{
			"type":      "function_call",
			"id":        tc.itemID,
			"call_id":   tc.id,
			"name":      name,
			"arguments": "",
			"status":    "in_progress",
		}
		doneItem := map[string]any{
			"type":      "function_call",
			"id":        tc.itemID,
			"call_id":   tc.id,
			"name":      name,
			"arguments": args,
			"status":    "completed",
		}
		if namespace != "" {
			addedItem["namespace"] = namespace
			doneItem["namespace"] = namespace
		}

		writeEvent("response.output_item.added", map[string]any{
			"output_index": outputIndex,
			"item":         addedItem,
		})

		if args != "{}" {
			writeEvent("response.function_call_arguments.delta", map[string]any{
				"item_id":      tc.itemID,
				"output_index": outputIndex,
				"delta":        args,
			})
		}

		writeEvent("response.output_item.done", map[string]any{
			"output_index": outputIndex,
			"item":         doneItem,
		})

		fcItems = append(fcItems, doneItem)
	}

	// Store reasoning for round-tripping
	reasoningStr := accumulatedReasoning.String()
	if reasoningStr != "" {
		for _, tc := range toolCalls {
			if tc.id != "" {
				sessions.StoreReasoning(tc.id, reasoningStr)
			}
		}
	}

	// Save session history
	assistantToolCalls := make([]json.RawMessage, 0, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		tc, ok := toolCalls[i]
		if !ok {
			continue
		}
		name := tc.name
		args := tc.arguments.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		tcJSON, _ := json.Marshal(map[string]any{
			"id":   tc.id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": args,
			},
		})
		assistantToolCalls = append(assistantToolCalls, tcJSON)
	}

	assistantMsg := ChatMessage{
		Role:    "assistant",
		Content: mustMarshalJSON(accumulatedText.String()),
	}
	if reasoningStr != "" {
		assistantMsg.ReasoningContent = &reasoningStr
	}
	if len(assistantToolCalls) > 0 {
		assistantMsg.ToolCalls = assistantToolCalls
	}

	// Store turn reasoning for content-based recovery
	if reasoningStr != "" {
		contentOnly := ChatMessage{
			Role:      "assistant",
			Content:   assistantMsg.Content,
			ToolCalls: assistantToolCalls,
		}
		sessions.StoreTurnReasoning(&contentOnly, reasoningStr)
	}

	// Build and save session messages from request
	var sessionMsgs []ChatMessage
	for _, reqMsg := range requestMessages {
		cm := mapToChatMessage(reqMsg)
		sessionMsgs = append(sessionMsgs, cm)
	}
	sessionMsgs = append(sessionMsgs, assistantMsg)
	sessions.SaveWithID(responseID, sessionMsgs)

	// Build output array for response.completed
	var outputItems []map[string]any
	if emittedMessageItem {
		outputItems = append(outputItems, map[string]any{
			"type":   "message",
			"id":     msgItemID,
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": accumulatedText.String()},
			},
		})
	}
	outputItems = append(outputItems, fcItems...)

	outputForCompleted := make([]any, len(outputItems))
	for i, item := range outputItems {
		outputForCompleted[i] = item
	}

	status := "completed"
	if streamFinishReason == "length" || streamFinishReason == "content_filter" {
		status = "incomplete"
	}
	completedEventResp := map[string]any{
		"id":     responseID,
		"status": status,
		"model":  requestModel,
		"output": outputForCompleted,
		"usage": map[string]any{
			"input_tokens":  streamUsage.prompt,
			"output_tokens": streamUsage.completion,
			"total_tokens":  streamUsage.total,
		},
	}
	if status == "incomplete" {
		completedEventResp["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
	}
	writeEvent("response.completed", map[string]any{
		"response": completedEventResp,
	})

	if canFlush {
		flusher.Flush()
	}

	return streamUsage.prompt, streamUsage.completion, streamUsage.total
}

func synthesizeResponsesSSE(w http.ResponseWriter, respBody []byte, flusher http.Flusher, canFlush bool, requestModel string, sessions *SessionStore, preResponseID string) (promptTokens, completionTokens, totalTokens int) {
	// Skip header setup if already sent by tryProvider (preResponseID != "")
	if preResponseID == "" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
	}

	seq := 0
	writeEvent := func(eventType string, data map[string]any) {
		seq++
		fieldsJSON, _ := jsonMarshalSafe(data)
		b := make([]byte, 0, len(fieldsJSON)+len(eventType)+12)
		b = append(b, `{"type":"`...)
		b = append(b, eventType...)
		b = append(b, `",`...)
		b = append(b, fieldsJSON[1:]...)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b); err != nil {
			slog.Error("synthesize-sse write error", "event", eventType, "error", err)
		}
		if canFlush {
			flusher.Flush()
		}
	}

	var respData struct {
		ID     string            `json:"id"`
		Model  string            `json:"model"`
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", respBody); err != nil {
			slog.Error("synthesize-sse raw write error", "error", err)
		}
		if canFlush {
			flusher.Flush()
		}

		return
	}

	// Skip response.created if already sent by tryProvider (preResponseID != "")
	if preResponseID == "" {
		writeEvent("response.created", map[string]any{
			"response": map[string]any{
				"id":     respData.ID,
				"status": "in_progress",
				"model":  requestModel,
			},
		})
	}

	var outputItems []any
	for i, rawItem := range respData.Output {
		var item map[string]any
		json.Unmarshal(rawItem, &item)
		outputItems = append(outputItems, item)

		addedItem := withStatus(item, "in_progress")
		if item["type"] == "message" {
			addedItem["content"] = []any{}
		}
		writeEvent("response.output_item.added", map[string]any{
			"output_index": i,
			"item":         addedItem,
		})

		if item["type"] == "message" {
			msgID, _ := item["id"].(string)
			contents, _ := item["content"].([]any)
			for _, c := range contents {
				cm, _ := c.(map[string]any)
				text, _ := cm["text"].(string)
				if text != "" {
					writeEvent("response.output_text.delta", map[string]any{
						"item_id":      msgID,
						"output_index": i,
						"delta":        text,
					})
				}
			}
		}

		if item["type"] == "function_call" {
			fcID, _ := item["id"].(string)
			callID, _ := item["call_id"].(string)
			args, _ := item["arguments"].(string)
			if args == "" {
				args = "{}"
			}
			writeEvent("response.function_call_arguments.delta", map[string]any{
				"item_id":      fcID,
				"output_index": i,
				"call_id":      callID,
				"delta":        args,
			})
			writeEvent("response.function_call_arguments.done", map[string]any{
				"item_id":      fcID,
				"output_index": i,
				"call_id":      callID,
				"arguments":    args,
			})
		}

		doneItem := withStatus(item, "completed")
		writeEvent("response.output_item.done", map[string]any{
			"output_index": i,
			"item":         doneItem,
		})
	}

	// Use request model in completed event
	writeEvent("response.completed", map[string]any{
		"response": map[string]any{
			"id":     respData.ID,
			"model":  requestModel,
			"status": respData.Status,
			"output": outputItems,
			"usage": map[string]any{
				"input_tokens":  respData.Usage.InputTokens,
				"output_tokens": respData.Usage.OutputTokens,
				"total_tokens":  respData.Usage.TotalTokens,
			},
		},
	})

	if canFlush {
		flusher.Flush()
	}

	return respData.Usage.InputTokens, respData.Usage.OutputTokens, respData.Usage.TotalTokens
}

func withStatus(item map[string]any, status string) map[string]any {
	cp := make(map[string]any, len(item))
	maps.Copy(cp, item)
	cp["status"] = status
	return cp
}

func isStreamingRequest(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	stream, _ := m["stream"].(bool)
	return stream
}

func extractSSEData(line string) (string, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	start := len(prefix)
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	return line[start:], true
}

func ensureStreamOptions(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if m["stream"] == true && m["stream_options"] == nil {
		m["stream_options"] = map[string]any{"include_usage": true}
		b, _ := json.Marshal(m)
		return b
	}
	return body
}

func sendResponseCreated(w http.ResponseWriter, responseID, requestModel string, writeMu *sync.Mutex) {
	data, _ := jsonMarshalSafe(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":     responseID,
			"status": "in_progress",
			"model":  requestModel,
		},
	})
	writeMu.Lock()
	if _, err := fmt.Fprintf(w, "event: response.created\ndata: %s\n\n", data); err != nil {
		slog.Error("response.created write error", "error", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	writeMu.Unlock()
}
