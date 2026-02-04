package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// Mock implementations

type mockMessageRepo struct {
	history  []domain.Message
	members  []domain.Member
	sentText []string
	mu       sync.Mutex
}

func (m *mockMessageRepo) GetChatHistory(ctx context.Context, chatID string, limit int) ([]domain.Message, error) {
	return m.history, nil
}

func (m *mockMessageRepo) GetChatMembers(ctx context.Context, chatID string) ([]domain.Member, error) {
	return m.members, nil
}

func (m *mockMessageRepo) GetChatInfo(ctx context.Context, chatID string) (*repo.ChatInfo, error) {
	return &repo.ChatInfo{ChatID: chatID, ChatType: domain.ChatTypeGroup}, nil
}

func (m *mockMessageRepo) SendText(ctx context.Context, chatID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentText = append(m.sentText, text)
	return nil
}

func (m *mockMessageRepo) SendTextWithMentions(ctx context.Context, chatID, text string, mentions []domain.Member) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentText = append(m.sentText, text)
	return nil
}

func (m *mockMessageRepo) AddReaction(ctx context.Context, msgID, reactionType string) error {
	return nil
}

type mockCodexRepo struct {
	threadID string
	turnID   string
	events   chan repo.Event
}

func (m *mockCodexRepo) CreateThread(ctx context.Context) (string, error) {
	return m.threadID, nil
}

func (m *mockCodexRepo) StartTurn(ctx context.Context, threadID, prompt string, images []string) (string, error) {
	return m.turnID, nil
}

func (m *mockCodexRepo) ResumeThread(ctx context.Context, threadID string) error {
	return nil
}

func (m *mockCodexRepo) Stop() {}

func (m *mockCodexRepo) Events() <-chan repo.Event {
	return m.events
}

type mockSessionRepo struct {
	sessions map[string]*domain.Session
	mu       sync.Mutex
}

func (m *mockSessionRepo) GetByChat(ctx context.Context, chatID string) (*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatID], nil
}

func (m *mockSessionRepo) Save(ctx context.Context, session *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[chatID(session)] = session
	return nil
}

func chatID(s *domain.Session) string {
	return s.ChatID
}

func (m *mockSessionRepo) Delete(ctx context.Context, chatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
	return nil
}

func (m *mockSessionRepo) Touch(ctx context.Context, chatID string) error {
	return nil
}

func (m *mockSessionRepo) MarkReplied(ctx context.Context, chatID string) error {
	return nil
}

func (m *mockSessionRepo) CleanupStale(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *mockSessionRepo) UpdateLastMsgTime(ctx context.Context, chatID string, msgTime time.Time) error {
	return nil
}

func (m *mockSessionRepo) UpdateLastProcessedMsg(ctx context.Context, chatID string, msgID string, msgTime time.Time) error {
	return nil
}

func (m *mockSessionRepo) ListAll(ctx context.Context) ([]*domain.Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) Close() error {
	return nil
}

type mockFilterRepo struct {
	shouldRespond bool
}

func (m *mockFilterRepo) ShouldRespond(ctx context.Context, message, history, strategy string) (bool, error) {
	return m.shouldRespond, nil
}

func (m *mockFilterRepo) SummarizeHistory(ctx context.Context, history string) (string, error) {
	return "", nil
}

// Tests

func TestParseResponse_PlainText(t *testing.T) {
	svc := &ConversationService{}
	text, mentions := svc.parseResponse("Hello, world!")

	if text != "Hello, world!" {
		t.Errorf("Expected 'Hello, world!', got '%s'", text)
	}
	if len(mentions) != 0 {
		t.Errorf("Expected 0 mentions, got %d", len(mentions))
	}
}

func TestParseResponse_WithReaction(t *testing.T) {
	svc := &ConversationService{}
	text, _ := svc.parseResponse("Thanks! [REACTION:ThumbsUp]")

	if text != "Thanks!" {
		t.Errorf("Expected 'Thanks!', got '%s'", text)
	}
}

func TestParseResponse_WithMention(t *testing.T) {
	svc := &ConversationService{}
	text, mentions := svc.parseResponse("Hello [MENTION:user123:张三]!")

	if text != "Hello !" {
		t.Errorf("Expected 'Hello !', got '%s'", text)
	}
	if len(mentions) != 1 {
		t.Fatalf("Expected 1 mention, got %d", len(mentions))
	}
	if mentions[0].UserID != "user123" {
		t.Errorf("Expected userID 'user123', got '%s'", mentions[0].UserID)
	}
	if mentions[0].Name != "张三" {
		t.Errorf("Expected name '张三', got '%s'", mentions[0].Name)
	}
}

func TestParseResponse_WithMentionAll(t *testing.T) {
	svc := &ConversationService{}
	text, _ := svc.parseResponse("Attention [MENTION_ALL] everyone!")

	if text != "Attention  everyone!" {
		t.Errorf("Expected 'Attention  everyone!', got '%s'", text)
	}
}

func TestParseResponse_Complex(t *testing.T) {
	svc := &ConversationService{}
	input := "Hi [MENTION:u1:Alice] and [MENTION:u2:Bob]! [REACTION:Wave] Great work!"
	text, mentions := svc.parseResponse(input)

	// Note: replacing [REACTION:Wave] leaves a space, resulting in double space
	expected := "Hi  and !  Great work!"
	if text != expected {
		t.Errorf("Expected '%s', got '%s'", expected, text)
	}
	if len(mentions) != 2 {
		t.Fatalf("Expected 2 mentions, got %d", len(mentions))
	}
}

func TestFindChatByThread(t *testing.T) {
	svc := &ConversationService{
		chatStates: make(map[string]*ChatState),
	}

	// Add a state
	svc.chatStates["chat-123"] = &ChatState{
		ThreadID: "thread-abc",
	}

	// Test finding existing
	chatID := svc.findChatByThread("thread-abc")
	if chatID != "chat-123" {
		t.Errorf("Expected 'chat-123', got '%s'", chatID)
	}

	// Test finding non-existing
	chatID = svc.findChatByThread("thread-xyz")
	if chatID != "" {
		t.Errorf("Expected empty string, got '%s'", chatID)
	}
}

func TestHandleCodexEvent_AgentDelta(t *testing.T) {
	svc := &ConversationService{
		chatStates: make(map[string]*ChatState),
	}

	// Setup a chat state
	svc.chatStates["chat-123"] = &ChatState{
		ThreadID: "thread-abc",
	}

	// Send delta event
	event := repo.Event{
		Type:     repo.EventTypeAgentDelta,
		ThreadID: "thread-abc",
		Data: &repo.AgentDeltaData{
			Delta: "Hello",
		},
	}
	svc.HandleCodexEvent(event)

	// Send another delta
	event2 := repo.Event{
		Type:     repo.EventTypeAgentDelta,
		ThreadID: "thread-abc",
		Data: &repo.AgentDeltaData{
			Delta: " World",
		},
	}
	svc.HandleCodexEvent(event2)

	// Check buffer
	state := svc.chatStates["chat-123"]
	if state.Buffer.String() != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", state.Buffer.String())
	}
}

func TestHandleCodexEvent_TurnComplete_SendsReply(t *testing.T) {
	msgRepo := &mockMessageRepo{}
	sessionRepo := &mockSessionRepo{sessions: make(map[string]*domain.Session)}
	codexRepo := &mockCodexRepo{events: make(chan repo.Event, 10)}

	// Create properly initialized usecase
	sessionCfg := domain.SessionConfig{IdleTimeout: 60 * time.Minute, ResetHour: 4}
	sessionUC := usecase.NewSessionUsecase(sessionRepo, codexRepo, sessionCfg)
	contextUC := usecase.NewContextBuilderUsecase(msgRepo)
	promptCfg := usecase.PromptConfig{}
	convUC := usecase.NewConversationUsecase(sessionUC, contextUC, codexRepo, promptCfg)

	svc := &ConversationService{
		chatStates:  make(map[string]*ChatState),
		messageRepo: msgRepo,
		convUC:      convUC,
	}

	var replyCalled bool
	var replyText string
	svc.SetReplyCallback(func(chatID, msgID, text string, mentions []domain.Member) {
		replyCalled = true
		replyText = text
	})

	// Setup a chat state with buffered content
	state := &ChatState{
		ThreadID: "thread-abc",
		MsgID:    "msg-123",
	}
	state.Buffer.WriteString("Test response")
	svc.chatStates["chat-123"] = state

	// Send turn complete event
	event := repo.Event{
		Type:     repo.EventTypeTurnComplete,
		ThreadID: "thread-abc",
	}
	svc.HandleCodexEvent(event)

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	if !replyCalled {
		t.Error("Expected reply callback to be called")
	}
	if replyText != "Test response" {
		t.Errorf("Expected 'Test response', got '%s'", replyText)
	}
}

func TestHandleCodexEvent_TurnComplete_EmptyBuffer(t *testing.T) {
	svc := &ConversationService{
		chatStates: make(map[string]*ChatState),
	}

	var replyCalled bool
	svc.SetReplyCallback(func(chatID, msgID, text string, mentions []domain.Member) {
		replyCalled = true
	})

	// Setup a chat state with EMPTY buffer
	svc.chatStates["chat-123"] = &ChatState{
		ThreadID: "thread-abc",
		MsgID:    "msg-123",
	}

	// Send turn complete event
	event := repo.Event{
		Type:     repo.EventTypeTurnComplete,
		ThreadID: "thread-abc",
	}
	svc.HandleCodexEvent(event)

	time.Sleep(50 * time.Millisecond)

	if replyCalled {
		t.Error("Reply should NOT be called for empty buffer")
	}
}

func TestHandleCodexEvent_UnknownThread(t *testing.T) {
	svc := &ConversationService{
		chatStates: make(map[string]*ChatState),
	}

	// Send event for unknown thread - should not panic
	event := repo.Event{
		Type:     repo.EventTypeAgentDelta,
		ThreadID: "unknown-thread",
		Data: &repo.AgentDeltaData{
			Delta: "test",
		},
	}
	svc.HandleCodexEvent(event)
	// No panic = pass
}

func TestGetChatState_CreatesNew(t *testing.T) {
	svc := &ConversationService{
		chatStates: make(map[string]*ChatState),
	}

	state := svc.getChatState("new-chat")

	if state == nil {
		t.Fatal("Expected non-nil state")
	}

	// Getting again should return same instance
	state2 := svc.getChatState("new-chat")
	if state != state2 {
		t.Error("Expected same state instance")
	}
}
