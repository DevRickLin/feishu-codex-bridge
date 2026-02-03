package bridge

import (
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		n        int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.n)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, result, tt.expected)
		}
	}
}

func TestChatState(t *testing.T) {
	state := &ChatState{}

	// Test initial state
	if state.Processing {
		t.Error("Initial Processing should be false")
	}
	if state.ThreadID != "" {
		t.Error("Initial ThreadID should be empty")
	}

	// Test setting values
	state.ThreadID = "test-thread"
	state.TurnID = "test-turn"
	state.Processing = true
	state.Buffer.WriteString("Hello ")
	state.Buffer.WriteString("World")

	if state.ThreadID != "test-thread" {
		t.Errorf("ThreadID mismatch: got %v", state.ThreadID)
	}
	if state.Buffer.String() != "Hello World" {
		t.Errorf("Buffer mismatch: got %v", state.Buffer.String())
	}

	// Test reset
	state.Buffer.Reset()
	if state.Buffer.String() != "" {
		t.Error("Buffer should be empty after reset")
	}
}

func TestConfig(t *testing.T) {
	config := Config{
		FeishuAppID:     "test-app-id",
		FeishuAppSecret: "test-secret",
		WorkingDir:      "/tmp/test",
		CodexModel:      "claude-sonnet-4",
		SessionDBPath:   "/tmp/sessions.db",
		SessionIdleMin:  60,
		SessionResetHr:  4,
		Debug:           true,
	}

	if config.FeishuAppID != "test-app-id" {
		t.Errorf("FeishuAppID mismatch: got %v", config.FeishuAppID)
	}
	if config.SessionIdleMin != 60 {
		t.Errorf("SessionIdleMin mismatch: got %v", config.SessionIdleMin)
	}
}
