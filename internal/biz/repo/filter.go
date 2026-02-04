package repo

import "context"

// FilterRepo is the message filtering interface
type FilterRepo interface {
	// ShouldRespond determines whether to respond
	// message: current message
	// history: recent chat history (formatted text)
	// strategy: custom strategy (optional, uses default if empty)
	ShouldRespond(ctx context.Context, message, history, strategy string) (bool, error)

	// SummarizeHistory summarizes chat history
	SummarizeHistory(ctx context.Context, history string) (string, error)
}
