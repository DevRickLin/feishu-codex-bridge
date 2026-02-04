package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"

	_ "modernc.org/sqlite"
)

// sessionRepo implements the Session repository
type sessionRepo struct {
	db *sql.DB
}

// NewSessionRepo creates a new Session repository
func NewSessionRepo(dbPath string) (repo.SessionRepo, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			chat_id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			last_reply_at INTEGER NOT NULL DEFAULT 0,
			last_msg_time INTEGER NOT NULL DEFAULT 0
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

	// Add last_msg_time column (if not exists) - for database migration
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN last_msg_time INTEGER NOT NULL DEFAULT 0`)

	// Add last_processed_msg_id column (if not exists) - for reliable message recovery
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN last_processed_msg_id TEXT NOT NULL DEFAULT ''`)

	return &sessionRepo{db: db}, nil
}

// GetByChat gets session by ChatID
func (r *sessionRepo) GetByChat(ctx context.Context, chatID string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT chat_id, thread_id, created_at, updated_at, last_reply_at, last_msg_time, last_processed_msg_id
		FROM sessions
		WHERE chat_id = ?
	`, chatID)

	var session domain.Session
	var createdAt, updatedAt, lastReplyAt, lastMsgTime int64
	err := row.Scan(&session.ChatID, &session.ThreadID, &createdAt, &updatedAt, &lastReplyAt, &lastMsgTime, &session.LastProcessedMsgID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	session.CreatedAt = time.Unix(createdAt, 0)
	session.UpdatedAt = time.Unix(updatedAt, 0)
	session.LastReplyAt = time.Unix(lastReplyAt, 0)
	session.LastMsgTime = time.Unix(lastMsgTime, 0)

	return &session, nil
}

// Save saves a session
func (r *sessionRepo) Save(ctx context.Context, session *domain.Session) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO sessions (chat_id, thread_id, created_at, updated_at, last_reply_at, last_msg_time, last_processed_msg_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		session.ChatID,
		session.ThreadID,
		session.CreatedAt.Unix(),
		session.UpdatedAt.Unix(),
		session.LastReplyAt.Unix(),
		session.LastMsgTime.Unix(),
		session.LastProcessedMsgID,
	)
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}

// Delete deletes a session
func (r *sessionRepo) Delete(ctx context.Context, chatID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE chat_id = ?`, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// Touch updates session active time
func (r *sessionRepo) Touch(ctx context.Context, chatID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sessions SET updated_at = ? WHERE chat_id = ?
	`, time.Now().Unix(), chatID)
	if err != nil {
		return fmt.Errorf("failed to touch session: %w", err)
	}
	return nil
}

// MarkReplied marks session as replied
func (r *sessionRepo) MarkReplied(ctx context.Context, chatID string) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		UPDATE sessions SET updated_at = ?, last_reply_at = ? WHERE chat_id = ?
	`, now, now, chatID)
	if err != nil {
		return fmt.Errorf("failed to mark replied: %w", err)
	}
	return nil
}

// UpdateLastMsgTime updates the last processed message time
func (r *sessionRepo) UpdateLastMsgTime(ctx context.Context, chatID string, msgTime time.Time) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		UPDATE sessions SET updated_at = ?, last_msg_time = ? WHERE chat_id = ?
	`, now, msgTime.Unix(), chatID)
	if err != nil {
		return fmt.Errorf("failed to update last msg time: %w", err)
	}
	return nil
}

// UpdateLastProcessedMsg updates the last processed message ID and time
func (r *sessionRepo) UpdateLastProcessedMsg(ctx context.Context, chatID string, msgID string, msgTime time.Time) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		UPDATE sessions SET updated_at = ?, last_msg_time = ?, last_processed_msg_id = ? WHERE chat_id = ?
	`, now, msgTime.Unix(), msgID, chatID)
	if err != nil {
		return fmt.Errorf("failed to update last processed msg: %w", err)
	}
	return nil
}

// CleanupStale cleans up stale sessions
func (r *sessionRepo) CleanupStale(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE updated_at < ?
	`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup stale sessions: %w", err)
	}
	return result.RowsAffected()
}

// ListAll lists all sessions
func (r *sessionRepo) ListAll(ctx context.Context) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT chat_id, thread_id, created_at, updated_at, last_reply_at, last_msg_time, last_processed_msg_id
		FROM sessions
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		var session domain.Session
		var createdAt, updatedAt, lastReplyAt, lastMsgTime int64
		if err := rows.Scan(&session.ChatID, &session.ThreadID, &createdAt, &updatedAt, &lastReplyAt, &lastMsgTime, &session.LastProcessedMsgID); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		session.CreatedAt = time.Unix(createdAt, 0)
		session.UpdatedAt = time.Unix(updatedAt, 0)
		session.LastReplyAt = time.Unix(lastReplyAt, 0)
		session.LastMsgTime = time.Unix(lastMsgTime, 0)
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// Close closes the database connection
func (r *sessionRepo) Close() error {
	return r.db.Close()
}
