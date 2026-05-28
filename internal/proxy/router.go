package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"relay-ai/internal/config"
)
// jsonMarshalSafe marshals JSON without HTML-escaping <, >, &.
// Go's json.Marshal HTML-escapes these by default, but codex-relay's serde_json
// does not. Codex CLI was tested against codex-relay output, and the escaping
// mismatch can cause parse failures in SSE event data during long responses.
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

// debugLog logs a message only when debug mode is enabled.
func debugLog(debug *atomic.Bool, format string, args ...any) {
	if debug != nil && debug.Load() {
		log.Printf(format, args...)
	}
}

func newRouter(store *config.Store, logger *Logger, sessions *SessionStore, debug *atomic.Bool) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/anthropic/{path...}", makeHandler(store, logger, sessions, cliClaude, "/anthropic", debug))
	mux.HandleFunc("/openai/{path...}", makeHandler(store, logger, sessions, cliCodex, "/openai", debug))
	mux.HandleFunc("/v1/responses", makeHandler(store, logger, sessions, cliCodex, "", debug))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return mux
}

func makeHandler(store *config.Store, logger *Logger, sessions *SessionStore, cliType, prefix string, debug *atomic.Bool) http.HandlerFunc {
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
				if len(p.CLITypes) == 0 || contains(p.CLITypes, cliType) {
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
			}, 0, 0, 0)
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
				}, 0, 0, 0)
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
			if strings.HasSuffix(upstreamPath, "/v1/responses") {
				upstreamPath = strings.TrimSuffix(upstreamPath, "/v1/responses") + "/v1/chat/completions"
			} else if strings.HasSuffix(upstreamPath, "/responses") {
				upstreamPath = strings.TrimSuffix(upstreamPath, "/responses") + "/chat/completions"
			}
			debugLog(debug, "[codex] path=%s chatCompatMode=true stream=%v body=%s", r.URL.Path, isStreamingRequest(bodyBytes), string(bodyBytes[:min(len(bodyBytes), 2000)]))
			bodyBytes, requestModel = toChatRequest(bodyBytes, sessions)
			debugLog(debug, "[codex] converted=%s", string(bodyBytes[:min(len(bodyBytes), 2000)]))

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
				log.Printf("proxy %s: invalid base_url %q: %v", cliType, provider.BaseURL, err)
				lastErr = fmt.Sprintf("invalid base_url: %v", err)
				continue
			}

			providerBody := transformBody(bodyBytes, &provider, debug)
			actualModel := extractModel(providerBody)

			upstreamURL := joinURL(target, upstreamPath)
			if r.URL.RawQuery != "" {
				upstreamURL += "?" + r.URL.RawQuery
			}

			debugLog(debug, "proxy %s [%d/%d] %s -> %s", cliType, i+1, len(providers), r.Method, upstreamURL)

			needResponseConversion := isResponsesPath && chatCompatMode
			canFallback := i < len(providers)-1
			result := tryProvider(w, r, upstreamURL, providerBody, &provider, canFallback, needResponseConversion, sessions, requestModel, requestMessages, debug)
			if result.StatusCode > 0 {
				logger.Add(RequestLog{
					Method:       r.Method,
					Path:         upstreamURL,
					UpstreamURL:  upstreamURL,
					CLIType:      cliType,
					ProviderID:   provider.ID,
					Provider:     provider.Name,
					Model:        actualModel,
					StatusCode:   result.StatusCode,
					Duration:     time.Since(start).Milliseconds(),
					ResponseBody: result.ResponseBody,
				}, result.PromptTokens, result.CompletionTokens, result.TotalTokens)
				return
			}

			lastErr = result.Error
			lastRespBody = result.ResponseBody
			lastUpstreamURL = upstreamURL
			log.Printf("proxy %s: provider %q failed, trying next", cliType, provider.Name)
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
		}, 0, 0, 0)
		http.Error(w, `{"error":{"message":"all providers failed for `+cliType+`"}}`, http.StatusBadGateway)
	}
}

// --- toChatRequest: Responses → Chat Completions (aligned with codex-relay translate.rs) ---

// toChatRequest converts a Responses API request to Chat Completions format.
// Returns the converted body and the request model name.
func toChatRequest(body []byte, sessions *SessionStore) ([]byte, string) {
	if len(body) == 0 {
		return body, ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body, ""
	}

	requestModel, _ := m["model"].(string)

	// --- Build messages ---
	var messages []map[string]any

	// Retrieve history from previous_response_id
	var history []ChatMessage
	if prevID, ok := m["previous_response_id"].(string); ok && prevID != "" && sessions != nil {
		history = sessions.GetHistory(prevID)
	}

	// Insert system/instructions at front
	systemText := ""
	if instructions, ok := m["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
		systemText = instructions
	}
	if systemText != "" {
		if len(history) == 0 || history[0].Role != "system" {
			messages = append(messages, map[string]any{
				"role":    "system",
				"content": systemText,
			})
		}
	}

	// Convert history ChatMessages back to maps
	historyCallIDs := make(map[string]bool)
	historyToolResponses := make(map[string]bool)
	for _, h := range history {
		if h.Role == "system" && messages[0]["role"] == "system" {
			// Skip duplicate system from history if we already added from instructions
			continue
		}
		conv := chatMessageToMap(h)
		messages = append(messages, conv)

		// Collect existing call_ids for dedup
		if h.Role == "assistant" {
			for _, tc := range h.ToolCalls {
				var callObj struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(tc, &callObj) == nil && callObj.ID != "" {
					historyCallIDs[callObj.ID] = true
				}
			}
		}
		if h.Role == "tool" && h.ToolCallID != nil {
			historyToolResponses[*h.ToolCallID] = true
		}
	}

	// Process input items
	input := m["input"]
	messages = appendInputItems(messages, input, historyCallIDs, historyToolResponses, sessions)

	// --- Tools conversion ---
	var toolsOut []any
	if tools, ok := m["tools"].([]any); ok {
		if converted := convertTools(tools); converted != nil {
			toolsOut = converted
		}
	}

	// --- Build output ---
	out := map[string]any{
		"model":    requestModel,
		"messages": messages,
	}
	if len(toolsOut) > 0 {
		out["tools"] = toolsOut
	}

	// Stream
	if stream, ok := m["stream"].(bool); ok {
		out["stream"] = stream
	}

	// Max tokens
	if v, ok := m["max_output_tokens"]; ok {
		n := toInt(v)
		if n > 0 {
			out["max_tokens"] = float64(n) // will be marshalled as number
		}
	}

	// Temperature
	if v, ok := m["temperature"]; ok {
		out["temperature"] = v
	}

	outBytes, err := json.Marshal(out)
	if err != nil {
		return body, requestModel
	}
	return outBytes, requestModel
}

// chatMessageToMap converts a stored ChatMessage back to a map for the outgoing request.
func chatMessageToMap(msg ChatMessage) map[string]any {
	m := map[string]any{
		"role": msg.Role,
	}
	if len(msg.Content) > 0 {
		var v any
		if json.Unmarshal(msg.Content, &v) == nil && v != nil {
			m["content"] = v
		}
	}
	if msg.ReasoningContent != nil {
		m["reasoning_content"] = *msg.ReasoningContent
	}
	if msg.ToolCallID != nil {
		m["tool_call_id"] = *msg.ToolCallID
	}
	if msg.Name != nil {
		m["name"] = *msg.Name
	}
	if len(msg.ToolCalls) > 0 {
		var toolCalls []any
		for _, tc := range msg.ToolCalls {
			var tcObj any
			if json.Unmarshal(tc, &tcObj) == nil {
				// Ensure function name has MCP namespace
				if tcMap, ok := tcObj.(map[string]any); ok {
					if fn, ok := tcMap["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok {
							fn["name"] = ensureMCPName(name)
						}
					}
				}
				toolCalls = append(toolCalls, tcObj)
			}
		}
		m["tool_calls"] = toolCalls
	}
	return m
}

// ensureMCPName ensures MCP function names use the proper namespace format.
// codex-relay splits mcp__server__fn → namespace=mcp__server__, name=fn.
// When replaying, we need to reconstruct the full name with namespace.
func ensureMCPName(name string) string {
	// Already has mcp__ prefix, use as-is
	if strings.HasPrefix(name, "mcp__") {
		return name
	}
	return name
}

// appendInputItems processes Responses API input items and appends them to messages.
// Aligned with codex-relay's to_chat_request input processing.
func appendInputItems(messages []map[string]any, input any, historyCallIDs, historyToolResponses map[string]bool, sessions *SessionStore) []map[string]any {
	if input == nil {
		return messages
	}

	// String input → single user message
	if s, ok := input.(string); ok {
		if strings.TrimSpace(s) != "" {
			messages = append(messages, map[string]any{
				"role":    "user",
				"content": s,
			})
		}
		return messages
	}

	items, ok := input.([]any)
	if !ok {
		return messages
	}

	i := 0
	for i < len(items) {
		item, ok := items[i].(map[string]any)
		if !ok {
			i++
			continue
		}

		itemType, _ := item["type"].(string)

		switch itemType {
		case "function_call":
			callID, _ := item["call_id"].(string)
			if historyCallIDs[callID] {
				i++
				continue // dedup: already in history
			}
			// Group consecutive function_call items into one assistant message
			var groupedCalls []map[string]any
			var reasoning string
			for j := i; j < len(items); j++ {
				cur, ok := items[j].(map[string]any)
				if !ok {
					break
				}
				if ct, _ := cur["type"].(string); ct != "function_call" {
					break
				}
				fcCallID, _ := cur["call_id"].(string)
				name := extractFunctionName(cur)
				args, _ := cur["arguments"].(string)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				groupedCalls = append(groupedCalls, map[string]any{
					"id":   fcCallID,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": args,
					},
				})
				if reasoning == "" && sessions != nil {
					reasoning = sessions.GetReasoning(fcCallID)
				}
				i++
			}
			assistantMsg := map[string]any{
				"role":       "assistant",
				"tool_calls": groupedCalls,
			}
			if reasoning != "" {
				assistantMsg["reasoning_content"] = reasoning
			}
			messages = append(messages, assistantMsg)
			continue // already advanced i inside the loop

		case "function_call_output":
			callID, _ := item["call_id"].(string)
			if historyToolResponses[callID] {
				i++
				continue // dedup
			}
			output := extractString(item["output"])
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      output,
			})
			i++

		case "reasoning":
			// Codex 0.128+ may replay reasoning items; drop them (handled via session store)
			i++

		default:
			// Regular message (user/assistant/developer)
			role, _ := item["role"].(string)
			if role == "developer" {
				role = "system"
			}
			if role == "" {
				role = "user"
			}

			content := convertContent(item["content"])
			msg := map[string]any{
				"role":    role,
				"content": content,
			}

			// For assistant messages, try to recover reasoning
			if role == "assistant" && sessions != nil {
				assistantCM := ChatMessage{
					Role:    "assistant",
					Content: mustMarshalJSON(content),
				}
				if r := sessions.GetTurnReasoning(&assistantCM); r != "" {
					msg["reasoning_content"] = r
				}
			}

			// System/developer messages must go to front
			if role == "system" {
				if len(messages) > 0 && messages[0]["role"] == "system" {
					messages[0] = msg
				} else {
					messages = append([]map[string]any{msg}, messages...)
				}
			} else {
				messages = append(messages, msg)
			}
			i++
		}
	}
	return messages
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

// extractFunctionName reconstructs the full Chat Completions function name
// from a Responses API function_call item, handling MCP namespaces.
func extractFunctionName(item map[string]any) string {
	name, _ := item["name"].(string)
	namespace, _ := item["namespace"].(string)
	return namespace + name
}

// --- fromChatResponse: Chat Completions → Responses API (aligned with codex-relay translate.rs) ---

// fromChatResponse converts a Chat Completions non-streaming response to Responses API format.
func fromChatResponse(body []byte, responseID, requestModel string, sessions *SessionStore) ([]byte, []ChatMessage) {
	if len(body) == 0 {
		return body, nil
	}

	var cc struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role             string  `json:"role"`
				Content          *string `json:"content"`
				ReasoningContent *string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &cc); err != nil {
		return body, nil
	}

	if cc.Error != nil {
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": cc.Error.Message,
				"type":    cc.Error.Type,
				"code":    cc.Error.Code,
			},
		})
		return b, nil
	}

	// Build output items
	var output []any
	var newMessages []ChatMessage

	if len(cc.Choices) > 0 {
		choice := cc.Choices[0]

		reasoningContent := ""
		textContent := ""
		if choice.Message.ReasoningContent != nil {
			reasoningContent = *choice.Message.ReasoningContent
		}
		if choice.Message.Content != nil {
			textContent = *choice.Message.Content
		}

		// Build assistant ChatMessage for session storage
		assistantMsg := ChatMessage{
			Role: "assistant",
		}
		if textContent != "" {
			assistantMsg.Content = mustMarshalJSON(textContent)
		}
		if reasoningContent != "" {
			assistantMsg.ReasoningContent = &reasoningContent
		}
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				tcJSON, _ := json.Marshal(map[string]any{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]any{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, tcJSON)
			}
		}
		newMessages = append(newMessages, assistantMsg)

		// Message output item (if text present or no tool calls)
		if textContent != "" || len(choice.Message.ToolCalls) == 0 {
			output = append(output, map[string]any{
				"type":   "message",
				"id":     fmt.Sprintf("msg_%s_0", responseID),
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{"type": "output_text", "text": textContent},
				},
			})
		}

		// Function call output items (with MCP namespace splitting)
		for i, tc := range choice.Message.ToolCalls {
			callID := tc.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%s_%d", responseID, i)
			}
			namespace, name := splitMCPName(tc.Function.Name)
			item := map[string]any{
				"type":      "function_call",
				"id":        fmt.Sprintf("fc_%s_%d", responseID, i),
				"call_id":   callID,
				"name":      name,
				"arguments": tc.Function.Arguments,
				"status":    "completed",
			}
			if namespace != "" {
				item["namespace"] = namespace
			}
			output = append(output, item)
		}
	}

	if len(output) == 0 {
		output = append(output, map[string]any{
			"type":   "message",
			"id":     fmt.Sprintf("msg_%s_0", responseID),
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": ""},
			},
		})
	}

	finishReason := "stop"
	if len(cc.Choices) > 0 && cc.Choices[0].FinishReason != nil {
		finishReason = *cc.Choices[0].FinishReason
	}

	status := "completed"
	var incompleteDetails map[string]any
	switch finishReason {
	case "length":
		status = "incomplete"
		incompleteDetails = map[string]any{"reason": "max_output_tokens"}
	case "content_filter":
		status = "incomplete"
		incompleteDetails = map[string]any{"reason": "content_filter"}
	}

	usage := map[string]any{
		"input_tokens":  0,
		"output_tokens": 0,
		"total_tokens":  0,
	}
	if cc.Usage != nil {
		usage = map[string]any{
			"input_tokens":  cc.Usage.PromptTokens,
			"output_tokens": cc.Usage.CompletionTokens,
			"total_tokens":  cc.Usage.TotalTokens,
		}
	}

	resp := map[string]any{
		"id":     responseID,
		"object": "response",
		"model":  requestModel,
		"status": status,
		"output": output,
		"usage":  usage,
	}
	if incompleteDetails != nil {
		resp["incomplete_details"] = incompleteDetails
	}

	b, _ := json.Marshal(resp)
	return b, newMessages
}

// splitMCPName splits an MCP function name like "mcp__server__fn" into (namespace, name).
// Returns (namespace, name) where namespace is like "mcp__server__" and name is the rest.
func splitMCPName(name string) (string, string) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", name
	}
	rest := name[len("mcp__"):]
	idx := strings.Index(rest, "__")
	if idx < 0 {
		return "", name
	}
	return name[:len("mcp__")+idx+2], name[len("mcp__")+idx+2:]
}

// --- translateStream: SSE Chat → Responses (aligned with codex-relay stream.rs) ---

// toolCallAccum accumulates tool call data from Chat Completions SSE deltas.
type toolCallAccum struct {
	id        string
	name      string
	arguments strings.Builder
	itemID    string
}

// translateStream converts an upstream Chat Completions SSE stream into Responses API SSE.
func translateStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher, canFlush bool, requestModel string, sessions *SessionStore, requestMessages []map[string]any, preResponseID string, keepAliveDone chan struct{}, writeMu *sync.Mutex, debug *atomic.Bool) (promptTokens, completionTokens, totalTokens int) {
	responseID := preResponseID
	if responseID == "" {
		responseID = sessions.NewID()
	}
	// msg_item_id uses an independent id, matching codex-relay (separate UUID)
	msgItemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	flusher, canFlush = ensureFlusher(w, flusher)
	headersSent := false

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
			debugLog(debug, "[codex-sse] -> event #%d: %s (data=%s)", seq, eventType, string(b[:min(len(b), 300)]))
		}
		writeMu.Lock()
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
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

	// 监听客户端断开信号
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	chunkCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractSSEData(line)
		if !ok {
			continue
		}
		if payload == "[DONE]" {
			streamDone = true
			debugLog(debug, "[codex-sse] received [DONE] after %d chunks", chunkCount)
			break
		}

		chunkCount++
		if chunkCount <= 3 {
			debugLog(debug, "[codex-sse] upstream chunk #%d: %s", chunkCount, payload[:min(len(payload), 300)])
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
			log.Printf("[codex-sse] chunk parse error at #%d: %v", chunkCount, err)
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


	// Check for scanner errors (upstream connection drop, timeout, etc.)
	if err := scanner.Err(); err != nil {
		log.Printf("[codex-sse] scanner error after %d chunks: %v", chunkCount, err)
	}
	// --- Finalize ---
	ensureHeaders()

	debugLog(debug, "[codex-sse] finalize: chunks=%d msgItemID=%s toolCalls=%d textLen=%d reasoningLen=%d usage=%+v",
		chunkCount, msgItemID, len(toolCalls), accumulatedText.Len(), accumulatedReasoning.Len(),
		map[string]int{"prompt": streamUsage.prompt, "completion": streamUsage.completion, "total": streamUsage.total})

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

func mapToChatMessage(m map[string]any) ChatMessage {
	cm := ChatMessage{}
	if r, ok := m["role"].(string); ok {
		cm.Role = r
	}
	if c, ok := m["content"]; ok {
		cm.Content = mustMarshalJSON(c)
	}
	if rc, ok := m["reasoning_content"].(string); ok && rc != "" {
		cm.ReasoningContent = &rc
	}
	if tci, ok := m["tool_call_id"].(string); ok && tci != "" {
		cm.ToolCallID = &tci
	}
	if n, ok := m["name"].(string); ok && n != "" {
		cm.Name = &n
	}
	if tcs, ok := m["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			tcJSON, _ := json.Marshal(tc)
			cm.ToolCalls = append(cm.ToolCalls, tcJSON)
		}
	}
	return cm
}

// --- synthesizeResponsesSSE: non-streaming → SSE (for errors / non-streaming fallback) ---

func synthesizeResponsesSSE(w http.ResponseWriter, respBody []byte, flusher http.Flusher, canFlush bool, requestModel string, sessions *SessionStore) (promptTokens, completionTokens, totalTokens int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	seq := 0
	writeEvent := func(eventType string, data map[string]any) {
		seq++
		fieldsJSON, _ := jsonMarshalSafe(data)
		b := make([]byte, 0, len(fieldsJSON)+len(eventType)+12)
		b = append(b, `{"type":"`...)
		b = append(b, eventType...)
		b = append(b, `",`...)
		b = append(b, fieldsJSON[1:]...)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
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
		fmt.Fprintf(w, "data: %s\n\n", respBody)
		if canFlush {
			flusher.Flush()
		}

		return
	}

	// Use request model, not upstream model
	writeEvent("response.created", map[string]any{
		"response": map[string]any{
			"id":     respData.ID,
			"status": "in_progress",
			"model":  requestModel,
		},
	})

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
	for k, v := range item {
		cp[k] = v
	}
	cp["status"] = status
	return cp
}

// --- Request forwarding ---

type tryProviderResult struct {
	StatusCode       int
	Error            string
	ResponseBody     string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// sendResponseCreated emits a response.created SSE event.
// Matches codex-relay: sent BEFORE upstream request so Codex knows the request was accepted.
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
	fmt.Fprintf(w, "event: response.created\ndata: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	writeMu.Unlock()
}

func tryProvider(w http.ResponseWriter, r *http.Request, upstreamURL string, body []byte, provider *config.Provider, canFallback bool, convertToResponses bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any, debug *atomic.Bool) tryProviderResult {
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
					fmt.Fprintf(w, ": keep-alive\n\n")
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					writeMu.Unlock()
				}
			}
		}()
	}

	// 创建自定义 http.Client，不设置响应头超时（深度思考可能需要很长时间）
	// 由请求 context 控制取消，避免上游长时间思考时连接被断开
	client := &http.Client{
		Transport: &http.Transport{},
	}
	upResp, err := client.Do(req)
	if err != nil {
		if keepAliveDone != nil {
			close(keepAliveDone)
		}
		return tryProviderResult{Error: fmt.Sprintf("upstream error: %v", err)}
	}
	defer upResp.Body.Close()

	if isStream {
		p, c, t := forwardStream(r.Context(), w, upResp, convertToResponses, sessions, requestModel, requestMessages, preResponseID, keepAliveDone, &writeMu, debug)
		if keepAliveDone != nil {
			close(keepAliveDone)
		}
		return tryProviderResult{StatusCode: upResp.StatusCode, PromptTokens: p, CompletionTokens: c, TotalTokens: t}
	}

	respBody, _ := io.ReadAll(upResp.Body)

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
	w.Write(respBody)
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
func forwardStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, convert bool, sessions *SessionStore, requestModel string, requestMessages []map[string]any, preResponseID string, keepAliveDone chan struct{}, writeMu *sync.Mutex, debug *atomic.Bool) (promptTokens, completionTokens, totalTokens int) {
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
			p, c, t := synthesizeResponsesSSE(w, convertedBody, flusher, canFlush, model, sessions)
			return p, c, t
		}
		// For streaming mode
		model := requestModel
		if model == "" {
			model = "unknown"
		}
		p, c, t := translateStream(ctx, w, resp, flusher, canFlush, model, sessions, requestMessages, preResponseID, keepAliveDone, writeMu, debug)
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
					fmt.Fprintf(w, ": keep-alive\n\n")
					flusher.Flush()
					writeMu.Unlock()
				}
			}
		}()
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(nil, 500*1024*1024)

	// 监听客户端断开信号
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	for scanner.Scan() {
		line := scanner.Text()
		writeMu.Lock()
		fmt.Fprintf(w, "%s\n", line)
		if line == "" {
			if canFlush {
				flusher.Flush()
			}
		}
		writeMu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		log.Printf("proxy: passthrough scanner error: %v", err)
	}
	if passthroughKeepAliveDone != nil {
		close(passthroughKeepAliveDone)
	}
	if canFlush {
		flusher.Flush()
	}
	return
}

// --- Utility functions ---

func ensureFlusher(w http.ResponseWriter, flusher http.Flusher) (http.Flusher, bool) {
	if flusher != nil {
		return flusher, true
	}
	if f, ok := w.(http.Flusher); ok {
		return f, true
	}
	return nil, false
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

// --- Request format conversion (for backward compat & non-chat mode) ---

func responsesToChat(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	b, _ := toChatRequest(body, nil)
	return b
}

func chatToResponses(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	b, _ := fromChatResponse(body, "resp_legacy", "unknown", nil)
	return b
}

func convertMessages(items []any) []any {
	var result []any
	var pendingToolCalls []map[string]any

	flushToolCalls := func() {
		if len(pendingToolCalls) == 0 {
			return
		}
		tcAny := make([]any, len(pendingToolCalls))
		for i, tc := range pendingToolCalls {
			tcAny[i] = tc
		}
		if len(result) > 0 {
			if lastMsg, ok := result[len(result)-1].(map[string]any); ok && lastMsg["role"] == "assistant" {
				if existing, _ := lastMsg["tool_calls"].([]any); existing != nil {
					lastMsg["tool_calls"] = append(existing, tcAny...)
				} else {
					lastMsg["tool_calls"] = tcAny
				}
				pendingToolCalls = nil
				return
			}
		}
		result = append(result, map[string]any{
			"role":       "assistant",
			"tool_calls": tcAny,
		})
		pendingToolCalls = nil
	}

	for _, item := range items {
		msgMap, ok := item.(map[string]any)
		if !ok {
			result = append(result, item)
			continue
		}

		msgType, _ := msgMap["type"].(string)
		role, _ := msgMap["role"].(string)

		switch msgType {
		case "function_call":
			callID, _ := msgMap["call_id"].(string)
			if callID == "" {
				callID, _ = msgMap["id"].(string)
			}
			name := extractFunctionName(msgMap)
			args, _ := msgMap["arguments"].(string)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			pendingToolCalls = append(pendingToolCalls, map[string]any{
				"id":   callID,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": args,
				},
			})

		case "function_call_output":
			flushToolCalls()
			callID, _ := msgMap["call_id"].(string)
			output := extractString(msgMap["output"])
			result = append(result, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      output,
			})

		default:
			flushToolCalls()
			if role == "developer" {
				role = "system"
				msgMap["role"] = "system"
			}
			msgMap["content"] = convertContent(msgMap["content"])
			delete(msgMap, "type")
			delete(msgMap, "status")
			delete(msgMap, "id")
			result = append(result, msgMap)
		}
	}
	flushToolCalls()
	return result
}

func extractString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []any:
		var parts []string
		for _, item := range val {
			if m, ok := item.(map[string]any); ok {
				t, _ := m["type"].(string)
				if t == "input_text" || t == "output_text" || t == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func convertContent(v any) any {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		allText := true
		var textParts []string
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				allText = false
				continue
			}
			t, _ := m["type"].(string)
			newType := mapContentType(t)
			m["type"] = newType
			if newType == "text" {
				if text, ok := m["text"].(string); ok {
					textParts = append(textParts, text)
				}
			} else {
				allText = false
			}
		}
		if allText && len(textParts) > 0 {
			return strings.Join(textParts, "")
		}
		return c
	case map[string]any:
		if t, ok := c["type"].(string); ok {
			c["type"] = mapContentType(t)
		}
		return c
	}
	return v
}

func mapContentType(t string) string {
	switch t {
	case "input_text", "output_text":
		return "text"
	case "input_image":
		return "image_url"
	case "input_file":
		return "file"
	}
	return t
}

func convertTools(tools []any) []any {
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		m, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if _, hasFn := m["function"]; hasFn {
			result = append(result, tool)
			continue
		}
		if t, _ := m["type"].(string); t != "function" {
			// Skip non-function tools (web_search, etc.) — many providers reject these
			continue
		}
		fn := map[string]any{}
		if v, ok := m["name"]; ok {
			fn["name"] = v
		}
		if v, ok := m["description"]; ok {
			fn["description"] = v
		}
		if v, ok := m["parameters"]; ok {
			fn["parameters"] = v
		}
		if v, ok := m["strict"]; ok {
			fn["strict"] = v
		}
		result = append(result, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func transformBody(body []byte, provider *config.Provider, debug *atomic.Bool) []byte {
	if len(body) == 0 {
		return body
	}
	if provider.DefaultModel == "" && len(provider.ModelMappings) == 0 {
		return body
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	currentModel, _ := m["model"].(string)
	if currentModel == "" {
		return body
	}
	newModel := ""
	for _, mapping := range provider.ModelMappings {
		if mapping.From == currentModel {
			newModel = mapping.To
			break
		}
	}
	if newModel == "" && provider.DefaultModel != "" {
		newModel = provider.DefaultModel
	}
	if newModel == "" || newModel == currentModel {
		return body
	}
	debugLog(debug, "proxy: model transform %q -> %q (provider: %s)", currentModel, newModel, provider.Name)
	m["model"] = newModel
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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
