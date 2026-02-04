package data

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/feishu-codex-bridge/codex"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

func TestConvertEvent_AgentMessageDelta(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	params := codex.AgentMessageDeltaParams{
		ThreadID: "thread-123",
		TurnID:   "turn-456",
		ItemID:   "item-789",
		Delta:    "Hello, world!",
	}
	paramsJSON, _ := json.Marshal(params)

	event := codex.Event{
		Method: codex.MethodAgentMessageDelta,
		Params: paramsJSON,
	}

	result := r.convertEvent(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Type != repo.EventTypeAgentDelta {
		t.Errorf("Expected type %s, got %s", repo.EventTypeAgentDelta, result.Type)
	}
	if result.ThreadID != "thread-123" {
		t.Errorf("Expected threadID 'thread-123', got '%s'", result.ThreadID)
	}
	if result.TurnID != "turn-456" {
		t.Errorf("Expected turnID 'turn-456', got '%s'", result.TurnID)
	}

	data, ok := result.Data.(*repo.AgentDeltaData)
	if !ok {
		t.Fatal("Expected AgentDeltaData")
	}
	if data.Delta != "Hello, world!" {
		t.Errorf("Expected delta 'Hello, world!', got '%s'", data.Delta)
	}
}

func TestConvertEvent_TurnCompleted(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	params := codex.TurnCompletedParams{
		ThreadID: "thread-abc",
		TurnID:   "turn-def",
		Status:   "completed",
	}
	paramsJSON, _ := json.Marshal(params)

	event := codex.Event{
		Method: codex.MethodTurnCompleted,
		Params: paramsJSON,
	}

	result := r.convertEvent(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Type != repo.EventTypeTurnComplete {
		t.Errorf("Expected type %s, got %s", repo.EventTypeTurnComplete, result.Type)
	}
	if result.ThreadID != "thread-abc" {
		t.Errorf("Expected threadID 'thread-abc', got '%s'", result.ThreadID)
	}
	if result.TurnID != "turn-def" {
		t.Errorf("Expected turnID 'turn-def', got '%s'", result.TurnID)
	}
}

func TestConvertEvent_ItemCompleted(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	params := codex.ItemCompletedParams{
		ThreadID: "thread-item",
		TurnID:   "turn-item",
	}
	paramsJSON, _ := json.Marshal(params)

	event := codex.Event{
		Method: codex.MethodItemCompleted,
		Params: paramsJSON,
	}

	result := r.convertEvent(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Type != repo.EventTypeItemCompleted {
		t.Errorf("Expected type %s, got %s", repo.EventTypeItemCompleted, result.Type)
	}
	if result.ThreadID != "thread-item" {
		t.Errorf("Expected threadID 'thread-item', got '%s'", result.ThreadID)
	}
}

func TestConvertEvent_InvalidJSON(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	event := codex.Event{
		Method: codex.MethodAgentMessageDelta,
		Params: json.RawMessage(`{invalid json}`),
	}

	result := r.convertEvent(event)

	if result != nil {
		t.Error("Expected nil result for invalid JSON")
	}
}

func TestConvertEvent_UnknownMethod(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	event := codex.Event{
		Method: "unknown/method",
		Params: json.RawMessage(`{}`),
	}

	result := r.convertEvent(event)

	if result != nil {
		t.Error("Expected nil result for unknown method")
	}
}

func TestConvertEvent_ErrorMethod(t *testing.T) {
	r := &codexRepo{
		eventsCh: make(chan repo.Event, 10),
	}

	event := codex.Event{
		Method: "some/error/event",
		Params: json.RawMessage(`{}`),
	}

	result := r.convertEvent(event)

	if result == nil {
		t.Fatal("Expected non-nil result for error method")
	}
	if result.Type != repo.EventTypeError {
		t.Errorf("Expected type %s, got %s", repo.EventTypeError, result.Type)
	}
}
