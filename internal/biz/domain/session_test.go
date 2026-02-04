package domain

import (
	"testing"
	"time"
)

func TestSession_IsFresh_WithinIdleTimeout(t *testing.T) {
	session := &Session{
		ChatID:    "chat-123",
		ThreadID:  "thread-456",
		CreatedAt: time.Now().Add(-30 * time.Minute),
		UpdatedAt: time.Now().Add(-10 * time.Minute),
	}

	cfg := SessionConfig{
		IdleTimeout: 30 * time.Minute,
		ResetHour:   -1, // disabled
	}

	if !session.IsFresh(cfg) {
		t.Error("Expected session to be fresh (within idle timeout)")
	}
}

func TestSession_IsFresh_ExceededIdleTimeout(t *testing.T) {
	session := &Session{
		ChatID:    "chat-123",
		ThreadID:  "thread-456",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	cfg := SessionConfig{
		IdleTimeout: 30 * time.Minute,
		ResetHour:   -1,
	}

	if session.IsFresh(cfg) {
		t.Error("Expected session to be stale (exceeded idle timeout)")
	}
}

func TestSession_IsFresh_ResetHour_CrossedToday(t *testing.T) {
	now := time.Now()
	resetHour := 4

	// Session updated before today's reset time
	resetTime := time.Date(now.Year(), now.Month(), now.Day(), resetHour, 0, 0, 0, now.Location())

	// If we're before reset time, test crossing yesterday's reset
	if now.Before(resetTime) {
		// Session updated before yesterday's reset
		yesterdayReset := resetTime.Add(-24 * time.Hour)
		session := &Session{
			ChatID:    "chat-123",
			ThreadID:  "thread-456",
			CreatedAt: yesterdayReset.Add(-1 * time.Hour),
			UpdatedAt: yesterdayReset.Add(-30 * time.Minute),
		}

		cfg := SessionConfig{
			IdleTimeout: 0, // disabled
			ResetHour:   resetHour,
		}

		if session.IsFresh(cfg) {
			t.Error("Expected session to be stale (crossed reset hour)")
		}
	} else {
		// We're after reset time, session updated before today's reset
		session := &Session{
			ChatID:    "chat-123",
			ThreadID:  "thread-456",
			CreatedAt: resetTime.Add(-2 * time.Hour),
			UpdatedAt: resetTime.Add(-1 * time.Hour),
		}

		cfg := SessionConfig{
			IdleTimeout: 0, // disabled
			ResetHour:   resetHour,
		}

		if session.IsFresh(cfg) {
			t.Error("Expected session to be stale (crossed reset hour)")
		}
	}
}

func TestSession_IsFresh_ResetHour_NotCrossed(t *testing.T) {
	now := time.Now()

	// Session updated recently (within same "day" period)
	session := &Session{
		ChatID:    "chat-123",
		ThreadID:  "thread-456",
		CreatedAt: now.Add(-1 * time.Hour),
		UpdatedAt: now.Add(-5 * time.Minute),
	}

	cfg := SessionConfig{
		IdleTimeout: 0, // disabled
		ResetHour:   4,
	}

	if !session.IsFresh(cfg) {
		t.Error("Expected session to be fresh (reset hour not crossed)")
	}
}

func TestSession_Touch(t *testing.T) {
	oldTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		ChatID:    "chat-123",
		ThreadID:  "thread-456",
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}

	session.Touch()

	if session.UpdatedAt.Before(oldTime.Add(time.Second)) {
		t.Error("Expected UpdatedAt to be updated")
	}
}

func TestSession_MarkReplied(t *testing.T) {
	oldTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		ChatID:      "chat-123",
		ThreadID:    "thread-456",
		CreatedAt:   oldTime,
		UpdatedAt:   oldTime,
		LastReplyAt: oldTime,
	}

	session.MarkReplied()

	if session.UpdatedAt.Before(oldTime.Add(time.Second)) {
		t.Error("Expected UpdatedAt to be updated")
	}
	if session.LastReplyAt.Before(oldTime.Add(time.Second)) {
		t.Error("Expected LastReplyAt to be updated")
	}
}
