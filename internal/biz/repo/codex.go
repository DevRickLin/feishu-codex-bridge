package repo

import (
	"context"
	"time"
)

// CodexRepo is the Codex interaction interface
type CodexRepo interface {
	// CreateThread creates a new Thread
	CreateThread(ctx context.Context) (threadID string, err error)

	// StartTurn starts a conversation turn
	StartTurn(ctx context.Context, threadID, prompt string, images []string) (turnID string, err error)

	// ResumeThread resumes a Thread (checks if it exists)
	ResumeThread(ctx context.Context, threadID string) error

	// Stop stops the Codex client
	Stop()

	// Events gets the event channel
	Events() <-chan Event

	// DebugConversation runs a complete conversation synchronously for debugging
	DebugConversation(ctx context.Context, prompt string, timeout time.Duration) (response string, threadID string, err error)
}

// Event represents a Codex event
type Event struct {
	Type     EventType
	ThreadID string
	TurnID   string
	Data     interface{}
}

// EventType represents the event type
type EventType string

const (
	EventTypeAgentDelta    EventType = "agent_delta"
	EventTypeTurnComplete  EventType = "turn_complete"
	EventTypeItemCompleted EventType = "item_completed"
	EventTypeError         EventType = "error"
)

// AgentDeltaData represents delta/incremental data
type AgentDeltaData struct {
	Delta string
}

// TurnCompleteData represents completion data
type TurnCompleteData struct {
	Response string
}

// ErrorData represents error data
type ErrorData struct {
	Error error
}
