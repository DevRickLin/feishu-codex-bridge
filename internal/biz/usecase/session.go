package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// SessionUsecase handles session logic
type SessionUsecase struct {
	sessionRepo repo.SessionRepo
	codexRepo   repo.CodexRepo
	config      domain.SessionConfig
}

// NewSessionUsecase creates a new session usecase
func NewSessionUsecase(
	sessionRepo repo.SessionRepo,
	codexRepo repo.CodexRepo,
	config domain.SessionConfig,
) *SessionUsecase {
	return &SessionUsecase{
		sessionRepo: sessionRepo,
		codexRepo:   codexRepo,
		config:      config,
	}
}

// ThreadDecision represents the thread decision result
type ThreadDecision struct {
	ThreadID           string
	IsNew              bool
	LastReplyAt        time.Time
	LastMsgTime        time.Time // Last processed message time (for disconnect recovery)
	LastProcessedMsgID string    // Last processed message ID (for reliable message recovery)
}

// ResolveThread resolves Thread (create or reuse)
func (uc *SessionUsecase) ResolveThread(ctx context.Context, chatID string) (*ThreadDecision, error) {
	session, err := uc.sessionRepo.GetByChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Need to create new Thread
	if session == nil || !session.IsFresh(uc.config) {
		return uc.createNewThread(ctx, chatID)
	}

	// Verify Thread exists
	if err := uc.codexRepo.ResumeThread(ctx, session.ThreadID); err != nil {
		// Thread lost, recreate
		_ = uc.sessionRepo.Delete(ctx, chatID)
		return uc.createNewThread(ctx, chatID)
	}

	return &ThreadDecision{
		ThreadID:           session.ThreadID,
		IsNew:              false,
		LastReplyAt:        session.LastReplyAt,
		LastMsgTime:        session.LastMsgTime,
		LastProcessedMsgID: session.LastProcessedMsgID,
	}, nil
}

func (uc *SessionUsecase) createNewThread(ctx context.Context, chatID string) (*ThreadDecision, error) {
	threadID, err := uc.codexRepo.CreateThread(ctx)
	if err != nil {
		return nil, fmt.Errorf("create thread: %w", err)
	}

	now := time.Now()
	session := &domain.Session{
		ChatID:    chatID,
		ThreadID:  threadID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := uc.sessionRepo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	return &ThreadDecision{
		ThreadID: threadID,
		IsNew:    true,
	}, nil
}

// MarkReplied marks session as replied
func (uc *SessionUsecase) MarkReplied(ctx context.Context, chatID string) error {
	return uc.sessionRepo.MarkReplied(ctx, chatID)
}

// Touch updates session active time
func (uc *SessionUsecase) Touch(ctx context.Context, chatID string) error {
	return uc.sessionRepo.Touch(ctx, chatID)
}

// GetSession gets a session
func (uc *SessionUsecase) GetSession(ctx context.Context, chatID string) (*domain.Session, error) {
	return uc.sessionRepo.GetByChat(ctx, chatID)
}

// UpdateLastMsgTime updates the last processed message time
func (uc *SessionUsecase) UpdateLastMsgTime(ctx context.Context, chatID string, msgTime time.Time) error {
	return uc.sessionRepo.UpdateLastMsgTime(ctx, chatID, msgTime)
}

// UpdateLastProcessedMsg updates the last processed message ID and time
func (uc *SessionUsecase) UpdateLastProcessedMsg(ctx context.Context, chatID string, msgID string, msgTime time.Time) error {
	return uc.sessionRepo.UpdateLastProcessedMsg(ctx, chatID, msgID, msgTime)
}
