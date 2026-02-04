package repo

import (
	"context"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
)

// BufferRepo is the message buffer repository interface
type BufferRepo interface {
	// Buffered message operations
	AddMessage(ctx context.Context, msg *domain.BufferedMessage) error
	GetUnprocessedMessages(ctx context.Context, chatID string) ([]*domain.BufferedMessage, error)
	GetAllUnprocessedMessages(ctx context.Context) ([]*domain.BufferedMessage, error)
	MarkProcessed(ctx context.Context, msgIDs []int64) error
	GetBufferSummary(ctx context.Context) ([]*domain.BufferSummary, error)
	CleanupOld(ctx context.Context, before time.Time) (int64, error)

	// Whitelist operations
	AddToWhitelist(ctx context.Context, entry *domain.WhitelistEntry) error
	RemoveFromWhitelist(ctx context.Context, chatID string) error
	GetWhitelist(ctx context.Context) ([]*domain.WhitelistEntry, error)
	IsInWhitelist(ctx context.Context, chatID string) (bool, error)

	// Keyword operations
	AddKeyword(ctx context.Context, kw *domain.TriggerKeyword) error
	RemoveKeyword(ctx context.Context, keyword string) error
	GetKeywords(ctx context.Context) ([]*domain.TriggerKeyword, error)
	MatchKeyword(ctx context.Context, content string) (*domain.TriggerKeyword, error)

	// Interest topic operations (for Moonshot filtering)
	AddInterestTopic(ctx context.Context, topic, description string) error
	RemoveInterestTopic(ctx context.Context, topic string) error
	GetInterestTopics(ctx context.Context) ([]string, error)

	Close() error
}
