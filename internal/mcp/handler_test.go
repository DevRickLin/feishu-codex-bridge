package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleToolCall_GetChatMembers(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/context":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id":   "test-chat",
				"chat_type": "group",
			})
		case "/api/chat/test-chat/members":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []Member{
					{ID: "u1", Name: "Alice"},
					{ID: "u2", Name: "Bob"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	result, err := handler.HandleToolCall("feishu_get_chat_members", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	members := resultMap["members"].([]Member)

	if len(members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(members))
	}
	if members[0].Name != "Alice" {
		t.Errorf("Expected first member Alice, got %s", members[0].Name)
	}
}

func TestHandleToolCall_GetChatHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/context":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id": "test-chat",
			})
		case "/api/chat/test-chat/history":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []Message{
					{ID: "m1", Content: "Hello"},
					{ID: "m2", Content: "World"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	result, err := handler.HandleToolCall("feishu_get_chat_history", map[string]interface{}{
		"limit": float64(10),
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	messages := resultMap["messages"].([]Message)

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}
}

func TestHandleToolCall_AddToWhitelist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/context":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id": "test-chat",
			})
		case "/api/whitelist":
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	result, err := handler.HandleToolCall("feishu_add_to_whitelist", map[string]interface{}{
		"reason": "important chat",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if !resultMap["success"].(bool) {
		t.Error("Expected success to be true")
	}
}

func TestHandleToolCall_AddKeyword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/context":
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case "/api/keywords":
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	result, err := handler.HandleToolCall("feishu_add_keyword", map[string]interface{}{
		"keyword":  "urgent",
		"priority": float64(2),
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if !resultMap["success"].(bool) {
		t.Error("Expected success to be true")
	}
}

func TestHandleToolCall_AddKeyword_MissingKeyword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/context" {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	_, err := handler.HandleToolCall("feishu_add_keyword", map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing keyword")
	}
}

func TestHandleToolCall_AddTopic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/context":
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case "/api/topics":
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handler := NewHandler(client)

	result, err := handler.HandleToolCall("feishu_add_interest_topic", map[string]interface{}{
		"topic": "PR review",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if !resultMap["success"].(bool) {
		t.Error("Expected success to be true")
	}
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	client := NewClient("http://localhost:9999")
	handler := NewHandler(client)

	_, err := handler.HandleToolCall("unknown_tool", nil)
	if err == nil {
		t.Error("Expected error for unknown tool")
	}
}

func TestGetStringArg(t *testing.T) {
	args := map[string]interface{}{
		"key1": "value1",
		"key2": "",
	}

	if v := getStringArg(args, "key1", "default"); v != "value1" {
		t.Errorf("Expected 'value1', got '%s'", v)
	}
	if v := getStringArg(args, "key2", "default"); v != "default" {
		t.Errorf("Expected 'default' for empty string, got '%s'", v)
	}
	if v := getStringArg(args, "key3", "default"); v != "default" {
		t.Errorf("Expected 'default' for missing key, got '%s'", v)
	}
}

func TestGetIntArg(t *testing.T) {
	args := map[string]interface{}{
		"float": float64(42),
		"int":   10,
	}

	if v := getIntArg(args, "float", 0); v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}
	if v := getIntArg(args, "int", 0); v != 10 {
		t.Errorf("Expected 10, got %d", v)
	}
	if v := getIntArg(args, "missing", 99); v != 99 {
		t.Errorf("Expected 99 for missing key, got %d", v)
	}
}

func TestFormatToolResult(t *testing.T) {
	result := FormatToolResult(map[string]string{"key": "value"}, false)

	if result["isError"].(bool) {
		t.Error("Expected isError to be false")
	}

	content := result["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("Expected type 'text', got '%s'", content[0]["type"])
	}

	// Error case
	errResult := FormatToolResult("error message", true)
	if !errResult["isError"].(bool) {
		t.Error("Expected isError to be true")
	}
}

func TestClient_GetContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/context" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(ChatContext{
				ChatID:    "chat-123",
				ChatType:  "group",
				MessageID: "msg-456",
				Members: []Member{
					{ID: "u1", Name: "Alice"},
				},
			})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, err := client.GetContext()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if ctx.ChatID != "chat-123" {
		t.Errorf("Expected chat_id 'chat-123', got '%s'", ctx.ChatID)
	}
	if ctx.ChatType != "group" {
		t.Errorf("Expected chat_type 'group', got '%s'", ctx.ChatType)
	}
	if len(ctx.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(ctx.Members))
	}
}

func TestGetToolDefinitions(t *testing.T) {
	tools := GetToolDefinitions()

	if len(tools) == 0 {
		t.Error("Expected non-empty tool definitions")
	}

	// Check that all tools have required fields
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("Tool missing name")
		}
		if tool.Description == "" {
			t.Errorf("Tool %s missing description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("Tool %s missing inputSchema", tool.Name)
		}
	}

	// Check specific tools exist
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{
		"feishu_get_chat_members",
		"feishu_get_chat_history",
		"feishu_add_to_whitelist",
		"feishu_remove_from_whitelist",
		"feishu_list_whitelist",
		"feishu_add_keyword",
		"feishu_remove_keyword",
		"feishu_list_keywords",
		"feishu_get_buffer_summary",
		"feishu_get_buffered_messages",
		"feishu_add_interest_topic",
		"feishu_remove_interest_topic",
		"feishu_list_interest_topics",
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("Missing expected tool: %s", name)
		}
	}
}
