package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"

	_ "modernc.org/sqlite"
)

// bufferRepo implements the message buffer repository
type bufferRepo struct {
	db *sql.DB
}

// NewBufferRepo creates a new message buffer repository
func NewBufferRepo(dbPath string) (repo.BufferRepo, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buffered messages table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS buffered_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id TEXT NOT NULL,
			msg_id TEXT UNIQUE NOT NULL,
			content TEXT NOT NULL,
			sender_id TEXT,
			sender_name TEXT,
			created_at INTEGER NOT NULL,
			processed INTEGER DEFAULT 0,
			processed_at INTEGER
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create buffered_messages table: %w", err)
	}

	// Create indexes
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_buffered_chat_processed ON buffered_messages(chat_id, processed)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_buffered_created ON buffered_messages(created_at)`)

	// Create whitelist table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS instant_whitelist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id TEXT UNIQUE NOT NULL,
			reason TEXT,
			added_by TEXT,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create instant_whitelist table: %w", err)
	}

	// Create keywords table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS trigger_keywords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT UNIQUE NOT NULL,
			priority INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create trigger_keywords table: %w", err)
	}

	// Create interest topics table (for Moonshot filtering)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS interest_topics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT UNIQUE NOT NULL,
			description TEXT,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create interest_topics table: %w", err)
	}

	fmt.Println("[Buffer] Database initialized")
	return &bufferRepo{db: db}, nil
}

// ========== Buffered Message Operations ==========

// AddMessage adds a message to the buffer
func (r *bufferRepo) AddMessage(ctx context.Context, msg *domain.BufferedMessage) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO buffered_messages (chat_id, msg_id, content, sender_id, sender_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, msg.ChatID, msg.MsgID, msg.Content, msg.SenderID, msg.SenderName, msg.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("failed to add buffered message: %w", err)
	}
	return nil
}

// GetUnprocessedMessages gets unprocessed messages for a specific chat
func (r *bufferRepo) GetUnprocessedMessages(ctx context.Context, chatID string) ([]*domain.BufferedMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chat_id, msg_id, content, sender_id, sender_name, created_at
		FROM buffered_messages
		WHERE chat_id = ? AND processed = 0
		ORDER BY created_at ASC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to query buffered messages: %w", err)
	}
	defer rows.Close()

	return scanBufferedMessages(rows)
}

// GetAllUnprocessedMessages gets all unprocessed messages
func (r *bufferRepo) GetAllUnprocessedMessages(ctx context.Context) ([]*domain.BufferedMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chat_id, msg_id, content, sender_id, sender_name, created_at
		FROM buffered_messages
		WHERE processed = 0
		ORDER BY chat_id, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all buffered messages: %w", err)
	}
	defer rows.Close()

	return scanBufferedMessages(rows)
}

func scanBufferedMessages(rows *sql.Rows) ([]*domain.BufferedMessage, error) {
	var messages []*domain.BufferedMessage
	for rows.Next() {
		var msg domain.BufferedMessage
		var createdAt int64
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.MsgID, &msg.Content, &msg.SenderID, &msg.SenderName, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan buffered message: %w", err)
		}
		msg.CreatedAt = time.Unix(createdAt, 0)
		messages = append(messages, &msg)
	}
	return messages, nil
}

// MarkProcessed marks messages as processed
func (r *bufferRepo) MarkProcessed(ctx context.Context, msgIDs []int64) error {
	if len(msgIDs) == 0 {
		return nil
	}

	// Build IN clause
	placeholders := make([]string, len(msgIDs))
	args := make([]interface{}, len(msgIDs)+1)
	args[0] = time.Now().Unix()
	for i, id := range msgIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf(`
		UPDATE buffered_messages
		SET processed = 1, processed_at = ?
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to mark messages processed: %w", err)
	}
	return nil
}

// GetBufferSummary gets buffer summary
func (r *bufferRepo) GetBufferSummary(ctx context.Context) ([]*domain.BufferSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT chat_id, COUNT(*) as msg_count, MAX(created_at) as last_msg
		FROM buffered_messages
		WHERE processed = 0
		GROUP BY chat_id
		ORDER BY last_msg DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query buffer summary: %w", err)
	}
	defer rows.Close()

	var summaries []*domain.BufferSummary
	for rows.Next() {
		var s domain.BufferSummary
		var lastMsg int64
		if err := rows.Scan(&s.ChatID, &s.MessageCount, &lastMsg); err != nil {
			return nil, fmt.Errorf("failed to scan buffer summary: %w", err)
		}
		s.LastMessage = time.Unix(lastMsg, 0)
		summaries = append(summaries, &s)
	}
	return summaries, nil
}

// CleanupOld cleans up old messages
func (r *bufferRepo) CleanupOld(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM buffered_messages WHERE created_at < ? AND processed = 1
	`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old messages: %w", err)
	}
	return result.RowsAffected()
}

// ========== Whitelist Operations ==========

// AddToWhitelist adds to whitelist
func (r *bufferRepo) AddToWhitelist(ctx context.Context, entry *domain.WhitelistEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO instant_whitelist (chat_id, reason, added_by, created_at)
		VALUES (?, ?, ?, ?)
	`, entry.ChatID, entry.Reason, entry.AddedBy, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to add to whitelist: %w", err)
	}
	fmt.Printf("[Buffer] Added %s to whitelist: %s\n", entry.ChatID, entry.Reason)
	return nil
}

// RemoveFromWhitelist removes from whitelist
func (r *bufferRepo) RemoveFromWhitelist(ctx context.Context, chatID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM instant_whitelist WHERE chat_id = ?`, chatID)
	if err != nil {
		return fmt.Errorf("failed to remove from whitelist: %w", err)
	}
	fmt.Printf("[Buffer] Removed %s from whitelist\n", chatID)
	return nil
}

// GetWhitelist gets the whitelist
func (r *bufferRepo) GetWhitelist(ctx context.Context) ([]*domain.WhitelistEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chat_id, reason, added_by, created_at
		FROM instant_whitelist
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query whitelist: %w", err)
	}
	defer rows.Close()

	var entries []*domain.WhitelistEntry
	for rows.Next() {
		var e domain.WhitelistEntry
		var createdAt int64
		if err := rows.Scan(&e.ID, &e.ChatID, &e.Reason, &e.AddedBy, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan whitelist entry: %w", err)
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		entries = append(entries, &e)
	}
	return entries, nil
}

// IsInWhitelist checks if chat is in whitelist
func (r *bufferRepo) IsInWhitelist(ctx context.Context, chatID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instant_whitelist WHERE chat_id = ?
	`, chatID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check whitelist: %w", err)
	}
	return count > 0, nil
}

// ========== Keyword Operations ==========

// AddKeyword adds a keyword
func (r *bufferRepo) AddKeyword(ctx context.Context, kw *domain.TriggerKeyword) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO trigger_keywords (keyword, priority, created_at)
		VALUES (?, ?, ?)
	`, kw.Keyword, kw.Priority, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to add keyword: %w", err)
	}
	fmt.Printf("[Buffer] Added keyword: %s (priority=%d)\n", kw.Keyword, kw.Priority)
	return nil
}

// RemoveKeyword removes a keyword
func (r *bufferRepo) RemoveKeyword(ctx context.Context, keyword string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM trigger_keywords WHERE keyword = ?`, keyword)
	if err != nil {
		return fmt.Errorf("failed to remove keyword: %w", err)
	}
	fmt.Printf("[Buffer] Removed keyword: %s\n", keyword)
	return nil
}

// GetKeywords gets all keywords
func (r *bufferRepo) GetKeywords(ctx context.Context) ([]*domain.TriggerKeyword, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, keyword, priority, created_at
		FROM trigger_keywords
		ORDER BY priority DESC, keyword ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query keywords: %w", err)
	}
	defer rows.Close()

	var keywords []*domain.TriggerKeyword
	for rows.Next() {
		var kw domain.TriggerKeyword
		var createdAt int64
		if err := rows.Scan(&kw.ID, &kw.Keyword, &kw.Priority, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan keyword: %w", err)
		}
		kw.CreatedAt = time.Unix(createdAt, 0)
		keywords = append(keywords, &kw)
	}
	return keywords, nil
}

// MatchKeyword matches a keyword
func (r *bufferRepo) MatchKeyword(ctx context.Context, content string) (*domain.TriggerKeyword, error) {
	keywords, err := r.GetKeywords(ctx)
	if err != nil {
		return nil, err
	}

	contentLower := strings.ToLower(content)
	for _, kw := range keywords {
		if strings.Contains(contentLower, strings.ToLower(kw.Keyword)) {
			return kw, nil
		}
	}
	return nil, nil
}

// ========== Interest Topic Operations ==========

// AddInterestTopic adds an interest topic
func (r *bufferRepo) AddInterestTopic(ctx context.Context, topic, description string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO interest_topics (topic, description, created_at)
		VALUES (?, ?, ?)
	`, topic, description, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to add interest topic: %w", err)
	}
	fmt.Printf("[Buffer] Added interest topic: %s\n", topic)
	return nil
}

// RemoveInterestTopic removes an interest topic
func (r *bufferRepo) RemoveInterestTopic(ctx context.Context, topic string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM interest_topics WHERE topic = ?`, topic)
	if err != nil {
		return fmt.Errorf("failed to remove interest topic: %w", err)
	}
	fmt.Printf("[Buffer] Removed interest topic: %s\n", topic)
	return nil
}

// GetInterestTopics gets all interest topics
func (r *bufferRepo) GetInterestTopics(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT topic FROM interest_topics ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query interest topics: %w", err)
	}
	defer rows.Close()

	var topics []string
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			return nil, fmt.Errorf("failed to scan interest topic: %w", err)
		}
		topics = append(topics, topic)
	}
	return topics, nil
}

// Close closes the database connection
func (r *bufferRepo) Close() error {
	return r.db.Close()
}
