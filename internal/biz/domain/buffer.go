package domain

import "time"

// BufferedMessage represents a buffered message
type BufferedMessage struct {
	ID          int64
	ChatID      string
	MsgID       string
	Content     string
	SenderID    string
	SenderName  string
	CreatedAt   time.Time
	Processed   bool
	ProcessedAt *time.Time
}

// WhitelistEntry represents an instant notification whitelist entry
type WhitelistEntry struct {
	ID        int64
	ChatID    string
	Reason    string
	AddedBy   string // "codex" or "user"
	CreatedAt time.Time
}

// TriggerKeyword represents a trigger keyword
type TriggerKeyword struct {
	ID        int64
	Keyword   string
	Priority  int // 1=normal, 2=high (triggers immediately)
	CreatedAt time.Time
}

// BufferSummary represents buffer overview
type BufferSummary struct {
	ChatID       string
	ChatName     string
	MessageCount int
	LastMessage  time.Time
}
