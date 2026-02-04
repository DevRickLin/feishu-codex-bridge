package domain

import (
	"testing"
	"time"
)

func TestConversation_IsGroup(t *testing.T) {
	groupConv := &Conversation{
		ChatType: ChatTypeGroup,
	}
	if !groupConv.IsGroup() {
		t.Error("Expected IsGroup() to return true for group chat")
	}

	p2pConv := &Conversation{
		ChatType: ChatTypeP2P,
	}
	if p2pConv.IsGroup() {
		t.Error("Expected IsGroup() to return false for P2P chat")
	}
}

func TestConversation_HistoryExcludingCurrent(t *testing.T) {
	now := time.Now()
	conv := &Conversation{
		History: []Message{
			{ID: "1", Content: "First", CreateTime: now.Add(-10 * time.Minute)},
			{ID: "2", Content: "Second", CreateTime: now.Add(-5 * time.Minute)},
			{ID: "3", Content: "Current", CreateTime: now},
		},
		Current: &Message{ID: "3", Content: "Current", CreateTime: now},
	}

	history := conv.HistoryExcludingCurrent()

	if len(history) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(history))
	}
	if history[0].ID != "1" || history[1].ID != "2" {
		t.Error("Expected messages 1 and 2")
	}
}

func TestConversation_HistoryExcludingCurrent_NoCurrent(t *testing.T) {
	now := time.Now()
	conv := &Conversation{
		History: []Message{
			{ID: "1", Content: "First", CreateTime: now.Add(-10 * time.Minute)},
			{ID: "2", Content: "Second", CreateTime: now.Add(-5 * time.Minute)},
		},
		Current: nil,
	}

	history := conv.HistoryExcludingCurrent()

	if len(history) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(history))
	}
}

func TestConversation_HistorySinceExcludingCurrent(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-8 * time.Minute)

	conv := &Conversation{
		History: []Message{
			{ID: "1", Content: "Old", CreateTime: now.Add(-15 * time.Minute)},
			{ID: "2", Content: "After cutoff 1", CreateTime: now.Add(-5 * time.Minute)},
			{ID: "3", Content: "After cutoff 2", CreateTime: now.Add(-2 * time.Minute)},
			{ID: "4", Content: "Current", CreateTime: now},
		},
		Current: &Message{ID: "4", Content: "Current", CreateTime: now},
	}

	history := conv.HistorySinceExcludingCurrent(cutoff)

	if len(history) != 2 {
		t.Fatalf("Expected 2 messages after cutoff (excluding current), got %d", len(history))
	}
	if history[0].ID != "2" {
		t.Errorf("Expected first message to be '2', got '%s'", history[0].ID)
	}
	if history[1].ID != "3" {
		t.Errorf("Expected second message to be '3', got '%s'", history[1].ID)
	}
}

func TestConversation_HistorySinceExcludingCurrent_NoRecentHistory(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)

	conv := &Conversation{
		History: []Message{
			{ID: "1", Content: "Old 1", CreateTime: now.Add(-15 * time.Minute)},
			{ID: "2", Content: "Old 2", CreateTime: now.Add(-10 * time.Minute)},
		},
		Current: &Message{ID: "3", Content: "Current", CreateTime: now},
	}

	history := conv.HistorySinceExcludingCurrent(cutoff)

	if len(history) != 0 {
		t.Fatalf("Expected 0 messages after cutoff, got %d", len(history))
	}
}
