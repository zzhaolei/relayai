package proxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"relay-ai/internal/config"
)

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
							fn["name"] = name // MCP namespace already in full name from history
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

func transformBody(body []byte, provider *config.Provider) []byte {
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
	m["model"] = newModel
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}
