package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test responsesToChat — Responses 请求 → Chat Completions 请求
// ---------------------------------------------------------------------------

func TestResponsesToChat_BasicText(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","instructions":"You are helpful.","input":"Hello"}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	messages, ok := m["messages"].([]any)
	if !ok {
		t.Fatal("messages not found in result")
	}
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}

	sysMsg, _ := messages[0].(map[string]any)
	if sysMsg["role"] != "system" {
		t.Errorf("expected system role, got %v", sysMsg["role"])
	}

	userMsg, _ := messages[1].(map[string]any)
	if userMsg["role"] != "user" {
		t.Errorf("expected user role, got %v", userMsg["role"])
	}
	if userMsg["content"] != "Hello" {
		t.Errorf("expected content 'Hello', got %v", userMsg["content"])
	}

	// Ensure Responses-only fields are removed
	for _, field := range []string{"input", "instructions", "include"} {
		if _, exists := m[field]; exists {
			t.Errorf("field %q should have been removed", field)
		}
	}
}

func TestResponsesToChat_NilInput(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","instructions":"You are helpful.","input":null}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	messages, ok := m["messages"].([]any)
	if !ok {
		t.Fatal("messages not found in result")
	}
	// Only system message, no user message for null input
	if len(messages) != 1 {
		t.Fatalf("expected 1 message (system only), got %d", len(messages))
	}
}

func TestResponsesToChat_EmptyInput(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","input":""}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Empty input produces nil messages (JSON null) which is fine
	messages := m["messages"]
	if messages != nil {
		if msgs, ok := messages.([]any); ok && len(msgs) > 0 {
			t.Fatalf("expected 0 messages for empty input, got %d", len(msgs))
		}
	}
}

func TestResponsesToChat_InputArray(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"Hi"},{"role":"assistant","content":"Hello!"}]}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	messages, ok := m["messages"].([]any)
	if !ok {
		t.Fatal("messages not found in result")
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestResponsesToChat_WithTools(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4o",
		"input":"call tool",
		"tools":[
			{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object"}}
		]
	}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	tools, ok := m["tools"].([]any)
	if !ok {
		t.Fatal("tools not found in result")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("expected tool type 'function', got %v", tool["type"])
	}
	fn := tool["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %v", fn["name"])
	}
}

func TestResponsesToChat_MaxOutputTokens(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","input":"Hi","max_output_tokens":500}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// max_output_tokens should be renamed to max_completion_tokens
	if _, exists := m["max_output_tokens"]; exists {
		t.Error("max_output_tokens should have been removed")
	}
	// toChatRequest uses max_tokens (matching codex-relay), not max_completion_tokens
	maxTokens, ok := m["max_tokens"].(float64)
	if !ok || int(maxTokens) != 500 {
		t.Errorf("expected max_tokens=500, got %v", m["max_tokens"])
	}
}

func TestResponsesToChat_FunctionCallMessages(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4o",
		"input":[
			{"role":"user","content":"call function"},
			{"type":"function_call","call_id":"call_1","name":"ping","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"pong"}
		]
	}`)
	result := responsesToChat(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	messages, ok := m["messages"].([]any)
	if !ok {
		t.Fatal("messages not found in result")
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Check assistant message with tool_calls
	assistantMsg, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatal("second message should be assistant")
	}
	if assistantMsg["role"] != "assistant" {
		t.Errorf("expected role assistant, got %v", assistantMsg["role"])
	}
	toolCalls, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatal("expected tool_calls in assistant message")
	}

	// Check tool message
	toolMsg, ok := messages[2].(map[string]any)
	if !ok {
		t.Fatal("third message should be tool")
	}
	if toolMsg["role"] != "tool" {
		t.Errorf("expected role tool, got %v", toolMsg["role"])
	}
	if toolMsg["content"] != "pong" {
		t.Errorf("expected content 'pong', got %v", toolMsg["content"])
	}
}

// ---------------------------------------------------------------------------
// Test chatToResponses — Chat Completions 响应 → Responses API 响应
// ---------------------------------------------------------------------------

func TestChatToResponses_BasicText(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-123","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"Hello, world!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	result := chatToResponses(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// chatToResponses uses resp_legacy for backward compat calls without sessions
	if m["id"] != "resp_legacy" {
		t.Errorf("expected id 'resp_legacy', got %v", m["id"])
	}
	if m["object"] != "response" {
		t.Errorf("expected object 'response', got %v", m["object"])
	}
	if m["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", m["status"])
	}

	output, ok := m["output"].([]any)
	if !ok {
		t.Fatal("output not found in result")
	}
	if len(output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(output))
	}
	item := output[0].(map[string]any)
	if item["type"] != "message" {
		t.Errorf("expected type 'message', got %v", item["type"])
	}
	content := item["content"].([]any)
	part := content[0].(map[string]any)
	if part["text"] != "Hello, world!" {
		t.Errorf("expected text 'Hello, world!', got %v", part["text"])
	}
}

func TestChatToResponses_ToolCalls(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-456","model":"gpt-4o","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_xyz","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	result := chatToResponses(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	output, ok := m["output"].([]any)
	if !ok {
		t.Fatal("output not found")
	}
	// Since content is nil, only function_call items should exist
	// But in our implementation, if content is nil AND there are no tool_calls, we add empty message
	// Here we have tool_calls, so we should have 1 function_call item
	// Actually wait: content is nil → no message item, but tool_calls exist → function_call items
	// But the condition is: if len(output) == 0, add empty message
	// Since we have tool_calls, len(output) > 0, so no empty message
	hasFuncCall := false
	for _, o := range output {
		item := o.(map[string]any)
		if item["type"] == "function_call" {
			hasFuncCall = true
			if item["call_id"] != "call_xyz" {
				t.Errorf("expected call_id 'call_xyz', got %v", item["call_id"])
			}
			if item["name"] != "get_weather" {
				t.Errorf("expected name 'get_weather', got %v", item["name"])
			}
		}
	}
	if !hasFuncCall {
		t.Error("expected function_call output item")
	}
}

func TestChatToResponses_ReasoningContent(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-789","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"The answer is 42.","reasoning_content":"I thought about it."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	result := chatToResponses(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	output, ok := m["output"].([]any)
	if !ok {
		t.Fatal("output not found")
	}
	// fromChatResponse matches codex-relay: reasoning is stored in session, 
	// not emitted as a separate output item. Only message output is present.
	if len(output) < 1 {
		t.Fatalf("expected at least 1 output item, got %d", len(output))
	}

	// Item should be message
	msgItem := output[0].(map[string]any)
	if msgItem["type"] != "message" {
		t.Errorf("expected type 'message', got %v", msgItem["type"])
	}
	contentPart := msgItem["content"].([]any)
	cp := contentPart[0].(map[string]any)
	if cp["text"] != "The answer is 42." {
		t.Errorf("expected text 'The answer is 42.', got %v", cp["text"])
	}
}

func TestChatToResponses_Incomplete(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-inc","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"partial..."},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
	result := chatToResponses(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if m["status"] != "incomplete" {
		t.Errorf("expected status 'incomplete', got %v", m["status"])
	}

	details, ok := m["incomplete_details"].(map[string]any)
	if !ok {
		t.Fatal("expected incomplete_details in response")
	}
	if details["reason"] != "max_output_tokens" {
		t.Errorf("expected reason 'max_output_tokens', got %v", details["reason"])
	}
}

func TestChatToResponses_Error(t *testing.T) {
	body := []byte(`{"error":{"message":"rate limit","type":"rate_limit","code":"429"}}`)
	result := chatToResponses(body)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["message"] != "rate limit" {
		t.Errorf("expected message 'rate limit', got %v", errObj["message"])
	}
}

// ---------------------------------------------------------------------------
// Test convertContent
// ---------------------------------------------------------------------------

func TestConvertContent_String(t *testing.T) {
	result := convertContent("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestConvertContent_Map(t *testing.T) {
	input := map[string]any{
		"type": "input_text",
		"text": "hello",
	}
	result := convertContent(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if m["type"] != "text" {
		t.Errorf("expected type 'text', got %v", m["type"])
	}
}

func TestConvertContent_AllTextArray_FlattenToStr(t *testing.T) {
	input := []any{
		map[string]any{"type": "input_text", "text": "Hello, "},
		map[string]any{"type": "output_text", "text": "world!"},
	}
	result := convertContent(input)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result for all-text array, got %T", result)
	}
	if s != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", s)
	}
}

func TestConvertContent_MixedArray_KeepArray(t *testing.T) {
	input := []any{
		map[string]any{"type": "input_text", "text": "Describe this image"},
		map[string]any{"type": "input_image", "image_url": "http://example.com/img.png"},
	}
	result := convertContent(input)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected array result for mixed array, got %T", result)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 items, got %d", len(arr))
	}
	// First item's type should be renamed
	item0 := arr[0].(map[string]any)
	if item0["type"] != "text" {
		t.Errorf("expected first item type 'text', got %v", item0["type"])
	}
	// Second item's type should be renamed
	item1 := arr[1].(map[string]any)
	if item1["type"] != "image_url" {
		t.Errorf("expected second item type 'image_url', got %v", item1["type"])
	}
}

func TestConvertContent_EmptyArray(t *testing.T) {
	input := []any{}
	result := convertContent(input)
	arr, ok := result.([]any)
	if !ok {
		t.Fatal("expected array result")
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d items", len(arr))
	}
}

// ---------------------------------------------------------------------------
// Test extractString
// ---------------------------------------------------------------------------

func TestExtractStringContent_String(t *testing.T) {
	result := extractString("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestExtractStringContent_TextArray(t *testing.T) {
	input := []any{
		map[string]any{"type": "input_text", "text": "image width: 100"},
		map[string]any{"type": "text", "text": "; image height: 200"},
	}
	result := extractString(input)
	if result != "image width: 100; image height: 200" {
		t.Errorf("expected concatenated text, got %q", result)
	}
}

func TestExtractStringContent_MixedArray(t *testing.T) {
	input := []any{
		map[string]any{"type": "text", "text": "image width: 100"},
		map[string]any{"type": "image_url", "image_url": "http://example.com/img.png"},
		map[string]any{"type": "text", "text": "; image height: 200"},
	}
	result := extractString(input)
	if result != "image width: 100; image height: 200" {
		t.Errorf("expected text-only concatenation, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Test convertMessages
// ---------------------------------------------------------------------------

func TestConvertMessages_DeveloperToSystem(t *testing.T) {
	input := []any{
		map[string]any{"role": "developer", "content": "You are helpful."},
	}
	result := convertMessages(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0].(map[string]any)
	if msg["role"] != "system" {
		t.Errorf("expected role 'system', got %v", msg["role"])
	}
}

func TestConvertMessages_FunctionCall(t *testing.T) {
	input := []any{
		map[string]any{"type": "function_call", "call_id": "call_1", "name": "ping", "arguments": `{"host":"x"}`},
	}
	result := convertMessages(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0].(map[string]any)
	if msg["role"] != "assistant" {
		t.Errorf("expected role 'assistant', got %v", msg["role"])
	}
	tcs := msg["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(tcs))
	}
}

func TestConvertMessages_FunctionCallOutput(t *testing.T) {
	input := []any{
		map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "pong"},
	}
	result := convertMessages(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0].(map[string]any)
	if msg["role"] != "tool" {
		t.Errorf("expected role 'tool', got %v", msg["role"])
	}
	if msg["content"] != "pong" {
		t.Errorf("expected content 'pong', got %v", msg["content"])
	}
}

func TestConvertMessages_FunctionCallOutput_Array(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_1",
			"output": []any{
				map[string]any{"type": "text", "text": "image width: 100"},
				map[string]any{"type": "image_url", "image_url": "http://example.com/img.png"},
				map[string]any{"type": "text", "text": "; image height: 200"},
			},
		},
	}
	result := convertMessages(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0].(map[string]any)
	if msg["role"] != "tool" {
		t.Errorf("expected role 'tool', got %v", msg["role"])
	}
	if msg["content"] != "image width: 100; image height: 200" {
		t.Errorf("expected flattened content, got %q", msg["content"])
	}
}

func TestConvertMessages_ContentFlattening(t *testing.T) {
	input := []any{
		map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "Part 1. "},
			map[string]any{"type": "input_text", "text": "Part 2."},
		}},
	}
	result := convertMessages(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0].(map[string]any)
	content, ok := msg["content"].(string)
	if !ok {
		t.Fatalf("expected string content, got %T", msg["content"])
	}
	if content != "Part 1. Part 2." {
		t.Errorf("expected 'Part 1. Part 2.', got %q", content)
	}
}

// ---------------------------------------------------------------------------
// Test convertTools
// ---------------------------------------------------------------------------

func TestConvertTools(t *testing.T) {
	input := []any{
		map[string]any{
			"type":        "function",
			"name":        "get_weather",
			"description": "Get weather",
			"parameters":  map[string]any{"type": "object"},
			"strict":      true,
		},
	}
	result := convertTools(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	tool := result[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("expected type 'function', got %v", tool["type"])
	}
	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatal("expected function map")
	}
	if fn["name"] != "get_weather" {
		t.Errorf("expected name 'get_weather', got %v", fn["name"])
	}
}

func TestConvertTools_AlreadyChatFormat(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "get_weather",
			},
		},
	}
	result := convertTools(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
}

func TestConvertTools_Empty(t *testing.T) {
	result := convertTools([]any{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// Test convertStreamSSE — Chat Completions SSE → Responses SSE
// ---------------------------------------------------------------------------

func buildSSEResponse(lines []string) *http.Response {
	sseData := ""
	for _, line := range lines {
		sseData += "data: " + line + "\n\n"
	}
	sseData += "data: [DONE]\n\n"
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sseData)),
	}
}


func captureSSEOutput(rec *httptest.ResponseRecorder) []string {
	var events []string
	body := rec.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			events = append(events, currentEvent+":"+strings.TrimPrefix(line, "data: "))
		}
	}
	return events
}

func TestConvertStreamSSE_PureText(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":", world!"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	p, c, total := translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	_ = p
	_ = c
	_ = total

	events := captureSSEOutput(rec)
	if len(events) == 0 {
		t.Fatal("expected SSE events")
	}

	// Verify event sequence matches codex-relay format
	hasCreated := false
	hasCompleted := false
	hasOutputItemDoneWithContent := false
	for _, evt := range events {
		if strings.Contains(evt, "response.created") {
			hasCreated = true
		}
		if strings.Contains(evt, "response.completed") {
			hasCompleted = true
		}
		// output_item.done must include full content (not separate output_text.done)
		if strings.Contains(evt, "response.output_item.done") && strings.Contains(evt, `"text":"Hello, world!"`) {
			hasOutputItemDoneWithContent = true
		}
	}
	if !hasCreated {
		t.Error("missing response.created event")
	}
	if !hasCompleted {
		t.Error("missing response.completed event")
	}
	if !hasOutputItemDoneWithContent {
		t.Error("missing output_item.done with full content text")
	}
}

func TestConvertStreamSSE_Reasoning(t *testing.T) {
	// 验证 reasoning_content 不发射 SSE 事件（对齐 codex-relay），content 正常发 output_text.delta
	chunks := []string{
		`{"id":"chatcmpl-r1","model":"gpt-4o","choices":[{"delta":{"role":"assistant","reasoning_content":"Let me think about this."},"finish_reason":null}]}`,
		`{"id":"chatcmpl-r1","model":"gpt-4o","choices":[{"delta":{"content":"Answer is 42."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	events := captureSSEOutput(rec)
	hasReasoningDelta := false
	hasTextDelta := false
	hasCompleted := false
	for _, evt := range events {
		if strings.Contains(evt, "response.reasoning_summary_text") {
			hasReasoningDelta = true
		}
		if strings.Contains(evt, "response.output_text.delta") && strings.Contains(evt, "Answer is 42") {
			hasTextDelta = true
		}
		if strings.Contains(evt, "response.completed") {
			hasCompleted = true
		}
	}
	if hasReasoningDelta {
		t.Error("reasoning should NOT emit SSE events (codex-relay does not)")
	}
	if !hasTextDelta {
		t.Error("missing output_text.delta for regular content")
	}
	if !hasCompleted {
		t.Error("missing response.completed")
	}
}

func TestConvertStreamSSE_ToolCalls(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"NYC\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	events := captureSSEOutput(rec)
	hasFuncCall := false
	for _, evt := range events {
		if strings.Contains(evt, "function_call") {
			hasFuncCall = true
		}
	}
	if !hasFuncCall {
		t.Error("missing function_call events")
	}
}

func TestConvertStreamSSE_Incomplete(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-inc","model":"gpt-4o","choices":[{"delta":{"role":"assistant","content":"partial..."},"finish_reason":null}]}`,
		`{"id":"chatcmpl-inc","model":"gpt-4o","choices":[{"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	events := captureSSEOutput(rec)
	hasIncomplete := false
	for _, evt := range events {
		if strings.Contains(evt, "incomplete_details") {
			hasIncomplete = true
		}
		if strings.Contains(evt, "\"status\":\"incomplete\"") {
			hasIncomplete = true
		}
	}
	if !hasIncomplete {
		t.Error("missing incomplete status/details in completed event")
	}
}


func TestConvertStreamSSE_ReasoningOnly(t *testing.T) {
	// 模拟纯推理模型：只有 reasoning_content，没有 content。
	// 对齐 codex-relay：reasoning 不发射 SSE 事件，仅内部累积。
	// 最终输出为空的 message item（因为模型只产推理不产实际回复）。
	chunks := []string{
		`{"id":"chatcmpl-ro","model":"mimo-v2.5-pro","choices":[{"delta":{"role":"assistant","reasoning_content":"The user just said"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-ro","model":"mimo-v2.5-pro","choices":[{"delta":{"reasoning_content":" \u4f60\u597d"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-ro","model":"mimo-v2.5-pro","choices":[{"delta":{"reasoning_content":" which means Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-ro","model":"mimo-v2.5-pro","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8289,"completion_tokens":94,"total_tokens":8383}}`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	events := captureSSEOutput(rec)

	hasReasoningEvent := false
	hasCompleted := false
	var completedBody string
	for _, evt := range events {
		if strings.Contains(evt, "response.reasoning_summary_text") {
			hasReasoningEvent = true
		}
		if strings.Contains(evt, "response.completed") {
			hasCompleted = true
			completedBody = evt
		}
	}

	if hasReasoningEvent {
		t.Error("reasoning should NOT emit SSE events (codex-relay does not)")
	}
	if !hasCompleted {
		t.Fatal("missing response.completed")
	}

	// 纯推理模型无实际 content 时，输出空 message（对齐 codex-relay）
	if !strings.Contains(completedBody, `"type":"message"`) {
		t.Error("completed output should contain message item")
	}
	// 推理文本不出现在输出中（仅在内部累积）
	fmt.Println("ReasoningOnly completed body (first 500):", completedBody[:min(len(completedBody), 500)])
}

func TestConvertStreamSSE_EmptyStream(t *testing.T) {
	chunks := []string{
		`[DONE]`,
	}
	resp := buildSSEResponse(chunks)
	rec := httptest.NewRecorder()
	p, c, total := translateStream(rec, resp, rec, true, "gpt-4o", NewSessionStore(), nil)

	if p != 0 || c != 0 || total != 0 {
		t.Errorf("expected zero tokens for empty stream, got p=%d c=%d t=%d", p, c, total)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "response.completed") {
		t.Error("expected response.completed event even for empty stream")
	}
	fmt.Println("Empty stream body:", body[:min(len(body), 500)])
}

// ---------------------------------------------------------------------------
// Test ensureStreamOptions
// ---------------------------------------------------------------------------

func TestEnsureStreamOptions_AddsIncludeUsage(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","stream":true}`)
	result := ensureStreamOptions(body)
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	opts, ok := m["stream_options"].(map[string]any)
	if !ok {
		t.Fatal("expected stream_options")
	}
	if opts["include_usage"] != true {
		t.Errorf("expected include_usage=true, got %v", opts["include_usage"])
	}
}

func TestEnsureStreamOptions_Preserves(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":true}}`)
	result := ensureStreamOptions(body)
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	opts := m["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Errorf("expected include_usage=true, got %v", opts["include_usage"])
	}
}

func TestEnsureStreamOptions_NonStream(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","stream":false}`)
	result := ensureStreamOptions(body)
	if len(result) != len(body) {
		t.Error("expected no change for non-stream request")
	}
}


