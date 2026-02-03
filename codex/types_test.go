package codex

import (
	"encoding/json"
	"testing"
)

func TestUserInputSerialization(t *testing.T) {
	tests := []struct {
		name     string
		input    UserInput
		expected string
	}{
		{
			name:     "text input",
			input:    UserInput{Type: "text", Text: "Hello world"},
			expected: `{"type":"text","text":"Hello world"}`,
		},
		{
			name:     "local image input",
			input:    UserInput{Type: "localImage", Path: "/path/to/image.png"},
			expected: `{"type":"localImage","path":"/path/to/image.png"}`,
		},
		{
			name:     "image URL input",
			input:    UserInput{Type: "image", URL: "https://example.com/image.png"},
			expected: `{"type":"image","url":"https://example.com/image.png"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			// Unmarshal expected to compare
			var expected, actual map[string]interface{}
			json.Unmarshal([]byte(tt.expected), &expected)
			json.Unmarshal(data, &actual)

			// Check type field
			if actual["type"] != expected["type"] {
				t.Errorf("Type mismatch: got %v, want %v", actual["type"], expected["type"])
			}
		})
	}
}

func TestTurnStartParamsSerialization(t *testing.T) {
	params := TurnStartParams{
		ThreadID: "test-thread-123",
		Input: []UserInput{
			{Type: "text", Text: "Hello"},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["threadId"] != "test-thread-123" {
		t.Errorf("threadId mismatch: got %v", result["threadId"])
	}

	input, ok := result["input"].([]interface{})
	if !ok || len(input) != 1 {
		t.Errorf("input mismatch: got %v", result["input"])
	}
}

func TestThreadStartResultDeserialization(t *testing.T) {
	// Simulated response from Codex
	jsonData := `{
		"thread": {
			"id": "abc123def456",
			"preview": "",
			"createdAt": 1234567890,
			"updatedAt": 1234567890,
			"cwd": "/home/user",
			"modelProvider": "anthropic",
			"cliVersion": "0.94.0",
			"source": "appServer",
			"turns": []
		}
	}`

	var result ThreadStartResult
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Thread.ID != "abc123def456" {
		t.Errorf("Thread ID mismatch: got %v, want abc123def456", result.Thread.ID)
	}
}

func TestRequestSerialization(t *testing.T) {
	req := Request{
		ID:     1,
		Method: "thread/start",
		Params: ThreadStartParams{},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["id"].(float64) != 1 {
		t.Errorf("id mismatch: got %v", result["id"])
	}
	if result["method"] != "thread/start" {
		t.Errorf("method mismatch: got %v", result["method"])
	}
}

func TestResponseDeserialization(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		hasErr  bool
		errCode int
	}{
		{
			name:   "success response",
			json:   `{"id": 1, "result": {"thread": {"id": "test"}}}`,
			hasErr: false,
		},
		{
			name:    "error response",
			json:    `{"id": 1, "error": {"code": -32600, "message": "Invalid request"}}`,
			hasErr:  true,
			errCode: -32600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp Response
			err := json.Unmarshal([]byte(tt.json), &resp)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if tt.hasErr {
				if resp.Error == nil {
					t.Error("Expected error but got nil")
				} else if resp.Error.Code != tt.errCode {
					t.Errorf("Error code mismatch: got %d, want %d", resp.Error.Code, tt.errCode)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Unexpected error: %v", resp.Error)
				}
			}
		})
	}
}

func TestNotificationDeserialization(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		method string
		hasID  bool
	}{
		{
			name:   "regular notification",
			json:   `{"method": "turn/completed", "params": {"threadId": "test"}}`,
			method: "turn/completed",
			hasID:  false,
		},
		{
			name:   "approval request (has ID)",
			json:   `{"id": 100, "method": "item/commandExecution/requestApproval", "params": {"command": "ls"}}`,
			method: "item/commandExecution/requestApproval",
			hasID:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notif Notification
			err := json.Unmarshal([]byte(tt.json), &notif)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if notif.Method != tt.method {
				t.Errorf("Method mismatch: got %v, want %v", notif.Method, tt.method)
			}

			if tt.hasID && notif.ID == 0 {
				t.Error("Expected ID but got 0")
			}
			if !tt.hasID && notif.ID != 0 {
				t.Errorf("Unexpected ID: %d", notif.ID)
			}
		})
	}
}

func TestAgentMessageDeltaParams(t *testing.T) {
	jsonStr := `{
		"threadId": "thread-1",
		"turnId": "turn-1",
		"itemId": "item-1",
		"delta": "Hello "
	}`

	var params AgentMessageDeltaParams
	err := json.Unmarshal([]byte(jsonStr), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if params.ThreadID != "thread-1" {
		t.Errorf("ThreadID mismatch: got %v", params.ThreadID)
	}
	if params.Delta != "Hello " {
		t.Errorf("Delta mismatch: got %v", params.Delta)
	}
}

func TestTurnCompletedParams(t *testing.T) {
	jsonStr := `{
		"threadId": "thread-1",
		"turnId": "turn-1",
		"status": "completed"
	}`

	var params TurnCompletedParams
	err := json.Unmarshal([]byte(jsonStr), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if params.Status != "completed" {
		t.Errorf("Status mismatch: got %v", params.Status)
	}
}
