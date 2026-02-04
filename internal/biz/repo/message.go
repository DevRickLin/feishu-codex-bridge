package repo

import (
	"context"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
)

// ChatInfo represents chat information
type ChatInfo struct {
	ChatID   string
	Name     string
	ChatType domain.ChatType
}

// MessageRepo is the message repository interface
// Responsible for fetching message data from Feishu API
type MessageRepo interface {
	// GetChatHistory gets chat history
	// Fetches in real-time from Feishu API, does not rely on local storage
	GetChatHistory(ctx context.Context, chatID string, limit int) ([]domain.Message, error)

	// GetChatMembers gets the list of chat members
	GetChatMembers(ctx context.Context, chatID string) ([]domain.Member, error)

	// GetChatInfo gets chat information
	GetChatInfo(ctx context.Context, chatID string) (*ChatInfo, error)

	// SendText sends a text message
	SendText(ctx context.Context, chatID, text string) error

	// SendTextWithMentions sends a text message with @ mentions
	SendTextWithMentions(ctx context.Context, chatID, text string, mentions []domain.Member) error

	// AddReaction adds an emoji reaction
	AddReaction(ctx context.Context, msgID, reactionType string) error
}
