package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// mockRoundTripper returns a fixed SSE response for any request.
type mockRoundTripper struct {
	chunks []string
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var sseBuf bytes.Buffer
	for _, chunk := range m.chunks {
		fmt.Fprintf(&sseBuf, "data: %s\n\n", chunk)
	}
	fmt.Fprintf(&sseBuf, "data: [DONE]\n\n")
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader(sseBuf.Bytes())),
		Request:    req,
	}, nil
}

// integrationTestWithTransport tests the full SSE conversion pipeline
// using a mock HTTP transport instead of a real server.
func integrationTestWithTransport(t *testing.T, upstreamChunks []string, responsesBody string) []string {
	t.Helper()

	sessions := NewSessionStore()

	// Save original transport and restore after test
	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = &mockRoundTripper{chunks: upstreamChunks}
	defer func() { http.DefaultClient.Transport = origTransport }()

	// Create a test HTTP server that runs the relayai handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()

		var reqMap map[string]any
		json.Unmarshal(bodyBytes, &reqMap)
		requestModel, _ := reqMap["model"].(string)

		// Convert to Chat Completions
		chatBody, _ := toChatRequest(bodyBytes, sessions)

		// Extract request messages
		var chatMap map[string]any
		json.Unmarshal(chatBody, &chatMap)
		var requestMessages []map[string]any
		if msgs, ok := chatMap["messages"].([]any); ok {
			for _, m := range msgs {
				if mm, ok := m.(map[string]any); ok {
					requestMessages = append(requestMessages, mm)
				}
			}
		}

		// Forward to mock upstream
		req, _ := http.NewRequest("POST", "http://mock/v1/chat/completions", bytes.NewReader(chatBody))
		req.Header.Set("Content-Type", "application/json")

		upResp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer upResp.Body.Close()

		flusher, canFlush := w.(http.Flusher)
		if requestModel == "" {
			requestModel = "gpt-4o"
		}
		translateStream(r.Context(), w, upResp, flusher, canFlush, requestModel, sessions, requestMessages, "", nil, new(sync.Mutex), new(atomic.Bool))
	})

	// Use httptest.NewUnstartedServer with a custom listener to avoid port bind
	// But actually, let's just test translateStream directly with a mock response

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/responses", strings.NewReader(responsesBody))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)

	// Capture SSE events
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

func TestIntegration_PureText(t *testing.T) {
	upstreamChunks := []string{
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":"Hello, world!"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}

	responsesBody := `{"model":"gpt-4o","instructions":"Be helpful.","input":"Hi","stream":true}`

	events := integrationTestWithTransport(t, upstreamChunks, responsesBody)

	t.Logf("Got %d events:", len(events))
	for i, e := range events {
		t.Logf("  [%d] %s", i, e[:min(len(e), 200)])
	}

	if len(events) == 0 {
		t.Fatal("no events received")
	}

	// Event 0: response.created
	if !strings.Contains(events[0], "response.created") {
		t.Fatal("first event should be response.created")
	}

	var createdData map[string]any
	json.Unmarshal([]byte(strings.SplitN(events[0], ":", 2)[1]), &createdData)
	resp, ok := createdData["response"].(map[string]any)
	if !ok {
		t.Fatal("response.created missing response object")
	}
	// Must have only: id, status, model (NO object, output, sequence_number)
	if _, hasObj := resp["object"]; hasObj {
		t.Error("response.created should NOT have 'object' field")
	}
	if _, hasOutput := resp["output"]; hasOutput {
		t.Error("response.created should NOT have 'output' field")
	}
	if _, hasSeq := resp["sequence_number"]; hasSeq {
		t.Error("response.created should NOT have 'sequence_number' field")
	}
	if resp["status"] != "in_progress" {
		t.Errorf("expected status 'in_progress', got %v", resp["status"])
	}
	if resp["model"] != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o' (request model), got %v", resp["model"])
	}
	id, _ := resp["id"].(string)
	if !strings.HasPrefix(id, "resp_") {
		t.Errorf("expected response.id to start with 'resp_', got %q", id)
	}
	t.Logf("✅ response.created id=%s model=%s", id, resp["model"])

	// Verify full event sequence
	foundAdded := false
	foundDelta := false
	foundDone := false
	foundCompleted := false
	for _, e := range events {
		if strings.Contains(e, "response.output_item.added") && strings.Contains(e, `"type":"message"`) {
			foundAdded = true
			var data map[string]any
			json.Unmarshal([]byte(strings.SplitN(e, ":", 2)[1]), &data)
			item, _ := data["item"].(map[string]any)
			content, _ := item["content"].([]any)
			if len(content) != 0 {
				t.Error("output_item.added content should be empty array []")
			}
		}
		if strings.Contains(e, "response.output_text.delta") && strings.Contains(e, "Hello, world!") {
			foundDelta = true
		}
		if strings.Contains(e, "response.output_item.done") && strings.Contains(e, `"text":"Hello, world!"`) {
			foundDone = true
		}
		if strings.Contains(e, "response.completed") {
			foundCompleted = true
			var data map[string]any
			json.Unmarshal([]byte(strings.SplitN(e, ":", 2)[1]), &data)
			resp, _ := data["response"].(map[string]any)
			if resp["model"] != "gpt-4o" {
				t.Errorf("response.completed model should be 'gpt-4o', got %v", resp["model"])
			}
			if resp["status"] != "completed" {
				t.Errorf("response.completed status should be 'completed', got %v", resp["status"])
			}
			// Verify full response.completed JSON is valid
			jsonBytes := []byte(strings.SplitN(e, ":", 2)[1])
			var full map[string]any
			if err := json.Unmarshal(jsonBytes, &full); err != nil {
				t.Errorf("response.completed JSON parse error: %v", err)
			}
			// type field must be first
			raw := string(jsonBytes[:min(len(jsonBytes), 100)])
			if !strings.HasPrefix(raw, `{"type":"response.completed"`) {
				t.Errorf("response.completed should start with type field, got: %s", raw)
			}
		}
	}
	if !foundAdded {
		t.Error("missing response.output_item.added")
	}
	if !foundDelta {
		t.Error("missing response.output_text.delta")
	}
	if !foundDone {
		t.Error("missing response.output_item.done with full text")
	}
	if !foundCompleted {
		t.Error("missing response.completed")
	}

	t.Log("✅ All required events present with correct format")
}

func TestIntegration_ResponseIDConsistency(t *testing.T) {
	upstreamChunks := []string{
		`{"id":"different-id-123","model":"gpt-4o","choices":[{"delta":{"content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	}

	responsesBody := `{"model":"gpt-4o","input":"Hello","stream":true}`

	events := integrationTestWithTransport(t, upstreamChunks, responsesBody)

	var createdID string
	for _, e := range events {
		if strings.Contains(e, "response.created") {
			var data map[string]any
			json.Unmarshal([]byte(strings.SplitN(e, ":", 2)[1]), &data)
			resp, _ := data["response"].(map[string]any)
			createdID, _ = resp["id"].(string)
		}
	}
	if createdID == "" {
		t.Fatal("could not extract response.id")
	}

	for _, e := range events {
		if strings.Contains(e, "response.completed") {
			var data map[string]any
			json.Unmarshal([]byte(strings.SplitN(e, ":", 2)[1]), &data)
			resp, _ := data["response"].(map[string]any)
			completedID, _ := resp["id"].(string)
			if completedID != createdID {
				t.Errorf("response.completed id %q != response.created id %q", completedID, createdID)
			}
			if completedID == "different-id-123" {
				t.Error("response.id should NOT be the upstream chunk ID")
			}
		}
	}

	t.Logf("✅ Response ID consistent: %s", createdID)
}

func TestIntegration_ModelField(t *testing.T) {
	upstreamChunks := []string{
		`{"id":"chatcmpl-1","model":"upstream-model-v2","choices":[{"delta":{"content":"test"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	}

	responsesBody := `{"model":"gpt-5.5","input":"test","stream":true}`

	events := integrationTestWithTransport(t, upstreamChunks, responsesBody)

	for _, e := range events {
		if strings.Contains(e, "response.created") || strings.Contains(e, "response.completed") {
			if strings.Contains(e, "upstream-model-v2") {
				t.Errorf("SSE event contains upstream model name, should use request model name:\n  %s", e[:min(len(e), 200)])
			}
		}
	}

	t.Log("✅ All SSE events use request model name")
}

func TestIntegration_ToolCalls(t *testing.T) {
	upstreamChunks := []string{
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"NYC\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","model":"gpt-4o","choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}

	responsesBody := `{"model":"gpt-4o","input":"What is the weather?","stream":true}`

	events := integrationTestWithTransport(t, upstreamChunks, responsesBody)

	t.Logf("Got %d events:", len(events))
	for i, e := range events {
		t.Logf("  [%d] %s", i, e[:min(len(e), 200)])
	}

	hasCreated := false
	hasFuncCallAdded := false
	hasFuncCallDelta := false
	hasFuncCallDone := false
	hasCompleted := false
	for _, e := range events {
		if strings.Contains(e, "response.created") {
			hasCreated = true
		}
		if strings.Contains(e, "response.output_item.added") && strings.Contains(e, "function_call") {
			hasFuncCallAdded = true
		}
		if strings.Contains(e, "response.function_call_arguments.delta") {
			hasFuncCallDelta = true
		}
		if strings.Contains(e, "response.output_item.done") && strings.Contains(e, "function_call") {
			hasFuncCallDone = true
		}
		if strings.Contains(e, "response.completed") {
			hasCompleted = true
		}
	}
	if !hasCreated {
		t.Error("missing response.created")
	}
	if !hasFuncCallAdded {
		t.Error("missing function_call output_item.added")
	}
	if !hasFuncCallDelta {
		t.Error("missing function_call_arguments.delta")
	}
	if !hasFuncCallDone {
		t.Error("missing function_call output_item.done")
	}
	if !hasCompleted {
		t.Error("missing response.completed")
	}

	t.Log("✅ Tool call SSE event sequence correct")
}
