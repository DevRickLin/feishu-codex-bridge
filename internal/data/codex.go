package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/feishu-codex-bridge/codex"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// codexRepo implements the Codex repository
type codexRepo struct {
	client   *codex.Client
	eventsCh chan repo.Event
}

// NewCodexRepo creates a new Codex repository
func NewCodexRepo(client *codex.Client) repo.CodexRepo {
	r := &codexRepo{
		client:   client,
		eventsCh: make(chan repo.Event, 100),
	}

	// Forward Codex events
	go r.forwardEvents()

	return r
}

// CreateThread creates a new Thread
func (r *codexRepo) CreateThread(ctx context.Context) (string, error) {
	return r.client.ThreadStart(ctx, nil)
}

// StartTurn starts a conversation turn
func (r *codexRepo) StartTurn(ctx context.Context, threadID, prompt string, images []string) (string, error) {
	return r.client.TurnStart(ctx, threadID, prompt, images)
}

// ResumeThread resumes a Thread
func (r *codexRepo) ResumeThread(ctx context.Context, threadID string) error {
	_, err := r.client.ThreadResume(ctx, threadID)
	return err
}

// Stop stops the client
func (r *codexRepo) Stop() {
	r.client.Stop()
	close(r.eventsCh)
}

// Events returns the event channel
func (r *codexRepo) Events() <-chan repo.Event {
	return r.eventsCh
}

// forwardEvents forwards Codex events
func (r *codexRepo) forwardEvents() {
	for event := range r.client.Events() {
		repoEvent := r.convertEvent(event)
		if repoEvent != nil {
			select {
			case r.eventsCh <- *repoEvent:
			default:
				// Channel full, drop event
			}
		}
	}
}

func (r *codexRepo) convertEvent(event codex.Event) *repo.Event {
	switch event.Method {
	case codex.MethodAgentMessageDelta:
		var params codex.AgentMessageDeltaParams
		if err := json.Unmarshal(event.Params, &params); err != nil {
			fmt.Printf("[CodexRepo] Failed to parse agent delta params: %v\n", err)
			return nil
		}
		return &repo.Event{
			Type:     repo.EventTypeAgentDelta,
			ThreadID: params.ThreadID,
			TurnID:   params.TurnID,
			Data: &repo.AgentDeltaData{
				Delta: params.Delta,
			},
		}

	case codex.MethodTurnCompleted:
		var params codex.TurnCompletedParams
		if err := json.Unmarshal(event.Params, &params); err != nil {
			fmt.Printf("[CodexRepo] Failed to parse turn completed params: %v\n", err)
			return nil
		}
		return &repo.Event{
			Type:     repo.EventTypeTurnComplete,
			ThreadID: params.ThreadID,
			TurnID:   params.TurnID,
			Data:     &repo.TurnCompleteData{},
		}

	case codex.MethodItemCompleted:
		var params codex.ItemCompletedParams
		if err := json.Unmarshal(event.Params, &params); err != nil {
			fmt.Printf("[CodexRepo] Failed to parse item completed params: %v\n", err)
			return nil
		}
		return &repo.Event{
			Type:     repo.EventTypeItemCompleted,
			ThreadID: params.ThreadID,
			TurnID:   params.TurnID,
		}

	default:
		if strings.Contains(string(event.Method), "error") {
			return &repo.Event{
				Type: repo.EventTypeError,
				Data: &repo.ErrorData{
					Error: fmt.Errorf("codex error: %s", event.Method),
				},
			}
		}
	}

	return nil
}
