package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Entry represents a session mapping between Feishu chat and Codex thread
type Entry struct {
	ChatID    string
	ThreadID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store manages session persistence using SQLite
type Store struct {
	db          *sql.DB
	idleMinutes int
	resetHour   int
}

// NewStore creates a new session store
func NewStore(dbPath string, idleMinutes, resetHour int) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			chat_id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	// Create index
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	return &Store{
		db:          db,
		idleMinutes: idleMinutes,
		resetHour:   resetHour,
	}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// GetByChatID retrieves a session by Feishu chat ID
func (s *Store) GetByChatID(chatID string) (*Entry, error) {
	row := s.db.QueryRow(`
		SELECT chat_id, thread_id, created_at, updated_at
		FROM sessions
		WHERE chat_id = ?
	`, chatID)

	var entry Entry
	var createdAt, updatedAt int64
	err := row.Scan(&entry.ChatID, &entry.ThreadID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	entry.CreatedAt = time.Unix(createdAt, 0)
	entry.UpdatedAt = time.Unix(updatedAt, 0)

	return &entry, nil
}

// Create creates a new session entry
func (s *Store) Create(chatID, threadID string) (*Entry, error) {
	now := time.Now()
	nowUnix := now.Unix()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO sessions (chat_id, thread_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, chatID, threadID, nowUnix, nowUnix)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Entry{
		ChatID:    chatID,
		ThreadID:  threadID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update updates the thread ID and timestamp for a session
func (s *Store) Update(chatID, threadID string) error {
	nowUnix := time.Now().Unix()

	_, err := s.db.Exec(`
		UPDATE sessions
		SET thread_id = ?, updated_at = ?
		WHERE chat_id = ?
	`, threadID, nowUnix, chatID)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// Touch updates the timestamp for a session (to track activity)
func (s *Store) Touch(chatID string) error {
	nowUnix := time.Now().Unix()

	_, err := s.db.Exec(`
		UPDATE sessions
		SET updated_at = ?
		WHERE chat_id = ?
	`, nowUnix, chatID)
	if err != nil {
		return fmt.Errorf("failed to touch session: %w", err)
	}

	return nil
}

// Delete removes a session
func (s *Store) Delete(chatID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE chat_id = ?`, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// IsFresh checks if a session is still fresh (not expired)
func (s *Store) IsFresh(entry *Entry) bool {
	if entry == nil {
		return false
	}

	now := time.Now()

	// Check idle timeout
	if s.idleMinutes > 0 {
		idleTime := now.Sub(entry.UpdatedAt)
		if idleTime > time.Duration(s.idleMinutes)*time.Minute {
			return false
		}
	}

	// Check daily reset (if configured)
	if s.resetHour >= 0 && s.resetHour < 24 {
		// Get today's reset time
		resetTime := time.Date(now.Year(), now.Month(), now.Day(), s.resetHour, 0, 0, 0, now.Location())

		// If current time is past reset and session was last updated before reset
		if now.After(resetTime) && entry.UpdatedAt.Before(resetTime) {
			return false
		}

		// If reset time is in the morning and session was from yesterday
		if now.Before(resetTime) {
			yesterdayReset := resetTime.Add(-24 * time.Hour)
			if entry.UpdatedAt.Before(yesterdayReset) {
				return false
			}
		}
	}

	return true
}

// CleanupStale removes all stale sessions
func (s *Store) CleanupStale() (int64, error) {
	if s.idleMinutes <= 0 {
		return 0, nil
	}

	cutoff := time.Now().Add(-time.Duration(s.idleMinutes) * time.Minute).Unix()
	result, err := s.db.Exec(`DELETE FROM sessions WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup stale sessions: %w", err)
	}

	return result.RowsAffected()
}

// ListAll returns all sessions (for debugging)
func (s *Store) ListAll() ([]*Entry, error) {
	rows, err := s.db.Query(`
		SELECT chat_id, thread_id, created_at, updated_at
		FROM sessions
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var entry Entry
		var createdAt, updatedAt int64
		if err := rows.Scan(&entry.ChatID, &entry.ThreadID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		entry.CreatedAt = time.Unix(createdAt, 0)
		entry.UpdatedAt = time.Unix(updatedAt, 0)
		entries = append(entries, &entry)
	}

	return entries, nil
}
