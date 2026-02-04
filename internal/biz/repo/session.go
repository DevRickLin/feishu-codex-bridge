package repo

import (
	"context"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
)

// SessionRepo is the session repository interface
// Responsible for session persistence (SQLite)
type SessionRepo interface {
	// GetByChat gets a session by ChatID
	GetByChat(ctx context.Context, chatID string) (*domain.Session, error)

	// Save saves a session (create or update)
	Save(ctx context.Context, session *domain.Session) error

	// Delete deletes a session
	Delete(ctx context.Context, chatID string) error

	// Touch updates session active time
	Touch(ctx context.Context, chatID string) error

	// MarkReplied marks session as replied
	MarkReplied(ctx context.Context, chatID string) error

	// UpdateLastMsgTime updates the last processed message time
	UpdateLastMsgTime(ctx context.Context, chatID string, msgTime time.Time) error

	// UpdateLastProcessedMsg updates the last processed message ID and time
	UpdateLastProcessedMsg(ctx context.Context, chatID string, msgID string, msgTime time.Time) error

	// CleanupStale cleans up stale sessions
	CleanupStale(ctx context.Context, before time.Time) (int64, error)

	// ListAll lists all sessions (for debugging)
	ListAll(ctx context.Context) ([]*domain.Session, error)
}
