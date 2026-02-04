package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// Mock implementations

type mockSessionRepo struct {
	sessions map[string]*domain.Session
}

func (m *mockSessionRepo) GetByChat(ctx context.Context, chatID string) (*domain.Session, error) {
	return m.sessions[chatID], nil
}

func (m *mockSessionRepo) Save(ctx context.Context, session *domain.Session) error {
	m.sessions[session.ChatID] = session
	return nil
}

func (m *mockSessionRepo) Delete(ctx context.Context, chatID string) error {
	delete(m.sessions, chatID)
	return nil
}

func (m *mockSessionRepo) Touch(ctx context.Context, chatID string) error {
	if s, ok := m.sessions[chatID]; ok {
		s.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockSessionRepo) MarkReplied(ctx context.Context, chatID string) error {
	if s, ok := m.sessions[chatID]; ok {
		now := time.Now()
		s.UpdatedAt = now
		s.LastReplyAt = now
	}
	return nil
}

func (m *mockSessionRepo) CleanupStale(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *mockSessionRepo) UpdateLastMsgTime(ctx context.Context, chatID string, msgTime time.Time) error {
	if s, ok := m.sessions[chatID]; ok {
		s.LastMsgTime = msgTime
		s.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockSessionRepo) UpdateLastProcessedMsg(ctx context.Context, chatID string, msgID string, msgTime time.Time) error {
	if s, ok := m.sessions[chatID]; ok {
		s.LastProcessedMsgID = msgID
		s.LastMsgTime = msgTime
		s.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockSessionRepo) ListAll(ctx context.Context) ([]*domain.Session, error) {
	var result []*domain.Session
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockSessionRepo) Close() error {
	return nil
}

type mockCodexRepo struct {
	threadCounter int
	events        chan repo.Event
}

func (m *mockCodexRepo) CreateThread(ctx context.Context) (string, error) {
	m.threadCounter++
	return "thread-" + string(rune('0'+m.threadCounter)), nil
}

func (m *mockCodexRepo) StartTurn(ctx context.Context, threadID, prompt string, images []string) (string, error) {
	return "turn-1", nil
}

func (m *mockCodexRepo) ResumeThread(ctx context.Context, threadID string) error {
	return nil
}

func (m *mockCodexRepo) Stop() {}

func (m *mockCodexRepo) Events() <-chan repo.Event {
	if m.events == nil {
		m.events = make(chan repo.Event, 10)
	}
	return m.events
}

// Tests

func TestResolveThread_NewSession(t *testing.T) {
	sessionRepo := &mockSessionRepo{sessions: make(map[string]*domain.Session)}
	codexRepo := &mockCodexRepo{}
	cfg := domain.SessionConfig{IdleTimeout: time.Hour, ResetHour: 4}

	uc := NewSessionUsecase(sessionRepo, codexRepo, cfg)

	decision, err := uc.ResolveThread(context.Background(), "chat-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !decision.IsNew {
		t.Error("Expected IsNew to be true")
	}
	if decision.ThreadID == "" {
		t.Error("Expected non-empty ThreadID")
	}

	// Verify session was saved
	if sessionRepo.sessions["chat-123"] == nil {
		t.Error("Expected session to be saved")
	}
}

func TestResolveThread_ExistingFreshSession(t *testing.T) {
	sessionRepo := &mockSessionRepo{sessions: make(map[string]*domain.Session)}
	codexRepo := &mockCodexRepo{}
	cfg := domain.SessionConfig{IdleTimeout: time.Hour, ResetHour: 4}

	// Create existing fresh session
	sessionRepo.sessions["chat-123"] = &domain.Session{
		ChatID:      "chat-123",
		ThreadID:    "existing-thread",
		CreatedAt:   time.Now().Add(-10 * time.Minute),
		UpdatedAt:   time.Now().Add(-5 * time.Minute),
		LastReplyAt: time.Now().Add(-5 * time.Minute),
	}

	uc := NewSessionUsecase(sessionRepo, codexRepo, cfg)

	decision, err := uc.ResolveThread(context.Background(), "chat-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if decision.IsNew {
		t.Error("Expected IsNew to be false")
	}
	if decision.ThreadID != "existing-thread" {
		t.Errorf("Expected 'existing-thread', got '%s'", decision.ThreadID)
	}
}

func TestResolveThread_StaleSession_IdleTimeout(t *testing.T) {
	sessionRepo := &mockSessionRepo{sessions: make(map[string]*domain.Session)}
	codexRepo := &mockCodexRepo{}
	cfg := domain.SessionConfig{IdleTimeout: 30 * time.Minute, ResetHour: -1}

	// Create stale session (idle too long)
	sessionRepo.sessions["chat-123"] = &domain.Session{
		ChatID:      "chat-123",
		ThreadID:    "old-thread",
		CreatedAt:   time.Now().Add(-2 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour), // 1 hour ago > 30 min timeout
		LastReplyAt: time.Now().Add(-1 * time.Hour),
	}

	uc := NewSessionUsecase(sessionRepo, codexRepo, cfg)

	decision, err := uc.ResolveThread(context.Background(), "chat-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !decision.IsNew {
		t.Error("Expected IsNew to be true for stale session")
	}
	if decision.ThreadID == "old-thread" {
		t.Error("Expected new thread ID, got old one")
	}
}

func TestMarkReplied(t *testing.T) {
	sessionRepo := &mockSessionRepo{sessions: make(map[string]*domain.Session)}
	codexRepo := &mockCodexRepo{}
	cfg := domain.SessionConfig{IdleTimeout: time.Hour, ResetHour: 4}

	// Create session
	oldTime := time.Now().Add(-10 * time.Minute)
	sessionRepo.sessions["chat-123"] = &domain.Session{
		ChatID:      "chat-123",
		ThreadID:    "thread-1",
		CreatedAt:   oldTime,
		UpdatedAt:   oldTime,
		LastReplyAt: oldTime,
	}

	uc := NewSessionUsecase(sessionRepo, codexRepo, cfg)

	err := uc.MarkReplied(context.Background(), "chat-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	session := sessionRepo.sessions["chat-123"]
	if session.UpdatedAt.Before(oldTime.Add(time.Second)) {
		t.Error("Expected UpdatedAt to be updated")
	}
	if session.LastReplyAt.Before(oldTime.Add(time.Second)) {
		t.Error("Expected LastReplyAt to be updated")
	}
}
