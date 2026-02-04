package domain

import "time"

// Session represents a session entity
type Session struct {
	ChatID             string
	ThreadID           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastReplyAt        time.Time // Bot's last reply time
	LastMsgTime        time.Time // Last processed message time (for disconnect recovery)
	LastProcessedMsgID string    // Last processed message ID (for reliable message recovery)
}

// SessionConfig represents session configuration (value object)
type SessionConfig struct {
	IdleTimeout time.Duration // Idle timeout
	ResetHour   int           // Daily reset hour (0-23, -1 to disable)
}

// IsFresh checks if session is valid
func (s *Session) IsFresh(cfg SessionConfig) bool {
	now := time.Now()

	// Check idle timeout
	if cfg.IdleTimeout > 0 {
		if now.Sub(s.UpdatedAt) > cfg.IdleTimeout {
			return false
		}
	}

	// Check daily reset
	if cfg.ResetHour >= 0 && cfg.ResetHour < 24 {
		resetTime := time.Date(now.Year(), now.Month(), now.Day(), cfg.ResetHour, 0, 0, 0, now.Location())

		if now.After(resetTime) && s.UpdatedAt.Before(resetTime) {
			return false
		}

		if now.Before(resetTime) {
			yesterdayReset := resetTime.Add(-24 * time.Hour)
			if s.UpdatedAt.Before(yesterdayReset) {
				return false
			}
		}
	}

	return true
}

// Touch updates active time
func (s *Session) Touch() {
	s.UpdatedAt = time.Now()
}

// MarkReplied marks as replied
func (s *Session) MarkReplied() {
	now := time.Now()
	s.UpdatedAt = now
	s.LastReplyAt = now
}

// UpdateLastMsgTime updates last processed message time
func (s *Session) UpdateLastMsgTime(t time.Time) {
	s.LastMsgTime = t
	s.UpdatedAt = time.Now()
}
