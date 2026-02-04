package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// MockMessageRepo implements repo.MessageRepo for testing
type MockMessageRepo struct {
	members  []domain.Member
	messages []domain.Message
}

func (m *MockMessageRepo) GetChatHistory(ctx context.Context, chatID string, limit int) ([]domain.Message, error) {
	if len(m.messages) > limit {
		return m.messages[:limit], nil
	}
	return m.messages, nil
}

func (m *MockMessageRepo) GetChatMembers(ctx context.Context, chatID string) ([]domain.Member, error) {
	return m.members, nil
}

func (m *MockMessageRepo) GetChatInfo(ctx context.Context, chatID string) (*repo.ChatInfo, error) {
	return &repo.ChatInfo{ChatID: chatID, ChatType: domain.ChatTypeGroup}, nil
}

func (m *MockMessageRepo) SendText(ctx context.Context, chatID, text string) error {
	return nil
}

func (m *MockMessageRepo) SendTextWithMentions(ctx context.Context, chatID, text string, mentions []domain.Member) error {
	return nil
}

func (m *MockMessageRepo) AddReaction(ctx context.Context, msgID, reactionType string) error {
	return nil
}

func TestHandleChatMembers(t *testing.T) {
	mockRepo := &MockMessageRepo{
		members: []domain.Member{
			{UserID: "u1", Name: "Alice"},
			{UserID: "u2", Name: "Bob"},
		},
	}

	server := &Server{
		messageRepo:    mockRepo,
		currentContext: &ChatContext{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/chat/test-chat/members", nil)
	w := httptest.NewRecorder()

	server.handleChatMembers(w, req, "test-chat")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string][]Member
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(result["members"]) != 2 {
		t.Errorf("Expected 2 members, got %d", len(result["members"]))
	}
	if result["members"][0].Name != "Alice" {
		t.Errorf("Expected first member Alice, got %s", result["members"][0].Name)
	}
}

func TestHandleChatHistory(t *testing.T) {
	mockRepo := &MockMessageRepo{
		messages: []domain.Message{
			{ID: "m1", Content: "Hello"},
			{ID: "m2", Content: "World"},
			{ID: "m3", Content: "Test"},
		},
	}

	server := &Server{
		messageRepo:    mockRepo,
		currentContext: &ChatContext{},
	}

	// Test with default limit
	req := httptest.NewRequest(http.MethodGet, "/api/chat/test-chat/history", nil)
	w := httptest.NewRecorder()

	server.handleChatHistory(w, req, "test-chat")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	messages := result["messages"].([]interface{})
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	// Test with limit
	req = httptest.NewRequest(http.MethodGet, "/api/chat/test-chat/history?limit=2", nil)
	w = httptest.NewRecorder()

	server.handleChatHistory(w, req, "test-chat")

	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	messages = result["messages"].([]interface{})
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages with limit, got %d", len(messages))
	}
}

func TestHandleContext(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{
			ChatID:   "chat-123",
			ChatType: "group",
			Members: []Member{
				{ID: "u1", Name: "Alice"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/context", nil)
	w := httptest.NewRecorder()

	server.handleContext(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var ctx ChatContext
	if err := json.Unmarshal(w.Body.Bytes(), &ctx); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
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

func TestSetAndGetContext(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{},
	}

	// Initially empty
	ctx := server.GetContext()
	if ctx.ChatID != "" {
		t.Error("Expected empty context initially")
	}

	// Set context
	server.SetContext(&ChatContext{
		ChatID:    "new-chat",
		ChatType:  "p2p",
		MessageID: "msg-1",
	})

	ctx = server.GetContext()
	if ctx.ChatID != "new-chat" {
		t.Errorf("Expected chat_id 'new-chat', got '%s'", ctx.ChatID)
	}
	if ctx.MessageID != "msg-1" {
		t.Errorf("Expected message_id 'msg-1', got '%s'", ctx.MessageID)
	}
}

func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("Expected 'ok', got '%s'", w.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{},
	}

	// POST to GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/context", nil)
	w := httptest.NewRecorder()

	server.handleContext(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestConvertMembers(t *testing.T) {
	members := []domain.Member{
		{UserID: "u1", Name: "Alice"},
		{UserID: "u2", Name: "Bob"},
	}

	result := ConvertMembers(members)

	if len(result) != 2 {
		t.Errorf("Expected 2 members, got %d", len(result))
	}
	if result[0].ID != "u1" || result[0].Name != "Alice" {
		t.Errorf("First member mismatch: %+v", result[0])
	}
}

// Test JSON encoding helper
func TestWriteJSON(t *testing.T) {
	server := &Server{}
	w := httptest.NewRecorder()

	data := map[string]string{"key": "value"}
	server.writeJSON(w, data)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json")
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("Expected key='value', got '%s'", result["key"])
	}
}

// Placeholder tests for buffer-related handlers (require BufferUsecase mock)
func TestHandleWhitelist_MethodNotAllowed(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/whitelist", nil)
	w := httptest.NewRecorder()

	server.handleWhitelist(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleKeywords_BadRequest(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{},
	}

	// POST without keyword
	body := bytes.NewBufferString(`{"priority": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/keywords", body)
	w := httptest.NewRecorder()

	server.handleKeywords(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleTopics_BadRequest(t *testing.T) {
	server := &Server{
		currentContext: &ChatContext{},
	}

	// POST without topic
	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/topics", body)
	w := httptest.NewRecorder()

	server.handleTopics(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
