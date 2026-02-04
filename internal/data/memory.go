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

// memoryRepo implements the memory repository
type memoryRepo struct {
	db *sql.DB
}

// NewMemoryRepo creates a new memory repository
func NewMemoryRepo(dbPath string) (repo.MemoryRepo, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create memories table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			content TEXT NOT NULL,
			category TEXT DEFAULT 'note',
			chat_id TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create memories table: %w", err)
	}

	// Create FTS virtual table for full-text search
	_, _ = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			key,
			content,
			category,
			content='memories',
			content_rowid='id'
		)
	`)

	// Create triggers for FTS sync
	_, _ = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, key, content, category)
			VALUES (new.id, new.key, new.content, new.category);
		END
	`)
	_, _ = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, content, category)
			VALUES ('delete', old.id, old.key, old.content, old.category);
		END
	`)
	_, _ = db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, content, category)
			VALUES ('delete', old.id, old.key, old.content, old.category);
			INSERT INTO memories_fts(rowid, key, content, category)
			VALUES (new.id, new.key, new.content, new.category);
		END
	`)

	// Create scheduled_tasks table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS scheduled_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			prompt TEXT NOT NULL,
			schedule_type TEXT NOT NULL,
			schedule_value TEXT NOT NULL,
			chat_id TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			next_run INTEGER,
			last_run INTEGER,
			last_status TEXT DEFAULT 'pending',
			last_error TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create scheduled_tasks table: %w", err)
	}

	// Create indexes for scheduled_tasks
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_next_run ON scheduled_tasks(next_run) WHERE enabled = 1`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_enabled ON scheduled_tasks(enabled)`)

	// Create heartbeat_configs table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS heartbeat_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id TEXT UNIQUE NOT NULL,
			interval_mins INTEGER NOT NULL DEFAULT 30,
			template TEXT,
			active_hours TEXT DEFAULT '00:00-23:59',
			timezone TEXT DEFAULT 'Asia/Shanghai',
			enabled INTEGER DEFAULT 1,
			last_heartbeat INTEGER,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create heartbeat_configs table: %w", err)
	}

	fmt.Println("[Memory] Database initialized")
	return &memoryRepo{db: db}, nil
}

// ========== Memory Operations ==========

func (r *memoryRepo) SaveMemory(ctx context.Context, entry *domain.MemoryEntry) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO memories (key, content, category, chat_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			content = excluded.content,
			category = excluded.category,
			chat_id = excluded.chat_id,
			updated_at = excluded.updated_at
	`, entry.Key, entry.Content, entry.Category, entry.ChatID, now, now)
	if err != nil {
		return fmt.Errorf("failed to save memory: %w", err)
	}
	fmt.Printf("[Memory] Saved: %s\n", entry.Key)
	return nil
}

func (r *memoryRepo) GetMemory(ctx context.Context, key string) (*domain.MemoryEntry, error) {
	var entry domain.MemoryEntry
	var createdAt, updatedAt int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, key, content, category, chat_id, created_at, updated_at
		FROM memories WHERE key = ?
	`, key).Scan(&entry.ID, &entry.Key, &entry.Content, &entry.Category, &entry.ChatID, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}
	entry.CreatedAt = time.Unix(createdAt, 0)
	entry.UpdatedAt = time.Unix(updatedAt, 0)
	return &entry, nil
}

func (r *memoryRepo) SearchMemory(ctx context.Context, query string, limit int) ([]*domain.MemoryEntry, error) {
	if limit <= 0 {
		limit = 10
	}

	// Try FTS search first, fall back to LIKE if FTS table doesn't exist
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.key, m.content, m.category, m.chat_id, m.created_at, m.updated_at
		FROM memories m
		JOIN memories_fts f ON m.id = f.rowid
		WHERE memories_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)

	if err != nil {
		// Fall back to LIKE search
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, key, content, category, chat_id, created_at, updated_at
			FROM memories
			WHERE key LIKE ? OR content LIKE ?
			ORDER BY updated_at DESC
			LIMIT ?
		`, "%"+query+"%", "%"+query+"%", limit)
		if err != nil {
			return nil, fmt.Errorf("failed to search memories: %w", err)
		}
	}
	defer rows.Close()

	return scanMemoryEntries(rows)
}

func (r *memoryRepo) ListMemories(ctx context.Context, category string, limit int) ([]*domain.MemoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error
	if category != "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, key, content, category, chat_id, created_at, updated_at
			FROM memories WHERE category = ?
			ORDER BY updated_at DESC
			LIMIT ?
		`, category, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, key, content, category, chat_id, created_at, updated_at
			FROM memories
			ORDER BY updated_at DESC
			LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	defer rows.Close()

	return scanMemoryEntries(rows)
}

func (r *memoryRepo) DeleteMemory(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM memories WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}
	fmt.Printf("[Memory] Deleted: %s\n", key)
	return nil
}

func scanMemoryEntries(rows *sql.Rows) ([]*domain.MemoryEntry, error) {
	var entries []*domain.MemoryEntry
	for rows.Next() {
		var entry domain.MemoryEntry
		var createdAt, updatedAt int64
		if err := rows.Scan(&entry.ID, &entry.Key, &entry.Content, &entry.Category, &entry.ChatID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan memory entry: %w", err)
		}
		entry.CreatedAt = time.Unix(createdAt, 0)
		entry.UpdatedAt = time.Unix(updatedAt, 0)
		entries = append(entries, &entry)
	}
	return entries, nil
}

// ========== Scheduled Task Operations ==========

func (r *memoryRepo) CreateTask(ctx context.Context, task *domain.ScheduledTask) error {
	now := time.Now().Unix()
	var nextRun interface{}
	if !task.NextRun.IsZero() {
		nextRun = task.NextRun.Unix()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			prompt = excluded.prompt,
			schedule_type = excluded.schedule_type,
			schedule_value = excluded.schedule_value,
			chat_id = excluded.chat_id,
			enabled = excluded.enabled,
			next_run = excluded.next_run,
			updated_at = excluded.updated_at
	`, task.Name, task.Prompt, task.ScheduleType, task.ScheduleValue, task.ChatID, task.Enabled, nextRun, now, now)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}
	fmt.Printf("[Memory] Created task: %s\n", task.Name)
	return nil
}

func (r *memoryRepo) GetTask(ctx context.Context, id int64) (*domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	var createdAt, updatedAt int64
	var nextRun, lastRun sql.NullInt64
	var lastError sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_run, last_status, last_error, created_at, updated_at
		FROM scheduled_tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.Name, &task.Prompt, &task.ScheduleType, &task.ScheduleValue, &task.ChatID, &task.Enabled, &nextRun, &lastRun, &task.LastStatus, &lastError, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	task.CreatedAt = time.Unix(createdAt, 0)
	task.UpdatedAt = time.Unix(updatedAt, 0)
	if nextRun.Valid {
		task.NextRun = time.Unix(nextRun.Int64, 0)
	}
	if lastRun.Valid {
		task.LastRun = time.Unix(lastRun.Int64, 0)
	}
	if lastError.Valid {
		task.LastError = lastError.String
	}
	return &task, nil
}

func (r *memoryRepo) GetTaskByName(ctx context.Context, name string) (*domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	var createdAt, updatedAt int64
	var nextRun, lastRun sql.NullInt64
	var lastError sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_run, last_status, last_error, created_at, updated_at
		FROM scheduled_tasks WHERE name = ?
	`, name).Scan(&task.ID, &task.Name, &task.Prompt, &task.ScheduleType, &task.ScheduleValue, &task.ChatID, &task.Enabled, &nextRun, &lastRun, &task.LastStatus, &lastError, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	task.CreatedAt = time.Unix(createdAt, 0)
	task.UpdatedAt = time.Unix(updatedAt, 0)
	if nextRun.Valid {
		task.NextRun = time.Unix(nextRun.Int64, 0)
	}
	if lastRun.Valid {
		task.LastRun = time.Unix(lastRun.Int64, 0)
	}
	if lastError.Valid {
		task.LastError = lastError.String
	}
	return &task, nil
}

func (r *memoryRepo) ListTasks(ctx context.Context, enabledOnly bool) ([]*domain.ScheduledTask, error) {
	var rows *sql.Rows
	var err error
	if enabledOnly {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_run, last_status, last_error, created_at, updated_at
			FROM scheduled_tasks WHERE enabled = 1
			ORDER BY next_run ASC
		`)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_run, last_status, last_error, created_at, updated_at
			FROM scheduled_tasks
			ORDER BY created_at DESC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	return scanScheduledTasks(rows)
}

func (r *memoryRepo) GetDueTasks(ctx context.Context, now time.Time) ([]*domain.ScheduledTask, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, prompt, schedule_type, schedule_value, chat_id, enabled, next_run, last_run, last_status, last_error, created_at, updated_at
		FROM scheduled_tasks
		WHERE enabled = 1 AND next_run <= ?
		ORDER BY next_run ASC
	`, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to get due tasks: %w", err)
	}
	defer rows.Close()

	return scanScheduledTasks(rows)
}

func (r *memoryRepo) UpdateTaskAfterRun(ctx context.Context, id int64, nextRun time.Time, status, errorMsg string) error {
	now := time.Now().Unix()
	var nextRunVal interface{}
	if !nextRun.IsZero() {
		nextRunVal = nextRun.Unix()
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET last_run = ?, next_run = ?, last_status = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`, now, nextRunVal, status, errorMsg, now, id)
	if err != nil {
		return fmt.Errorf("failed to update task after run: %w", err)
	}
	return nil
}

func (r *memoryRepo) EnableTask(ctx context.Context, id int64, enabled bool) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_tasks SET enabled = ?, updated_at = ? WHERE id = ?
	`, enabled, now, id)
	if err != nil {
		return fmt.Errorf("failed to enable/disable task: %w", err)
	}
	return nil
}

func (r *memoryRepo) DeleteTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}
	return nil
}

func scanScheduledTasks(rows *sql.Rows) ([]*domain.ScheduledTask, error) {
	var tasks []*domain.ScheduledTask
	for rows.Next() {
		var task domain.ScheduledTask
		var createdAt, updatedAt int64
		var nextRun, lastRun sql.NullInt64
		var lastError sql.NullString
		if err := rows.Scan(&task.ID, &task.Name, &task.Prompt, &task.ScheduleType, &task.ScheduleValue, &task.ChatID, &task.Enabled, &nextRun, &lastRun, &task.LastStatus, &lastError, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		task.CreatedAt = time.Unix(createdAt, 0)
		task.UpdatedAt = time.Unix(updatedAt, 0)
		if nextRun.Valid {
			task.NextRun = time.Unix(nextRun.Int64, 0)
		}
		if lastRun.Valid {
			task.LastRun = time.Unix(lastRun.Int64, 0)
		}
		if lastError.Valid {
			task.LastError = lastError.String
		}
		tasks = append(tasks, &task)
	}
	return tasks, nil
}

// ========== Heartbeat Operations ==========

func (r *memoryRepo) SetHeartbeat(ctx context.Context, config *domain.HeartbeatConfig) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO heartbeat_configs (chat_id, interval_mins, template, active_hours, timezone, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			interval_mins = excluded.interval_mins,
			template = excluded.template,
			active_hours = excluded.active_hours,
			timezone = excluded.timezone,
			enabled = excluded.enabled
	`, config.ChatID, config.IntervalMins, config.Template, config.ActiveHours, config.Timezone, config.Enabled, now)
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}
	fmt.Printf("[Memory] Set heartbeat for chat %s: every %d mins\n", config.ChatID, config.IntervalMins)
	return nil
}

func (r *memoryRepo) GetHeartbeat(ctx context.Context, chatID string) (*domain.HeartbeatConfig, error) {
	var config domain.HeartbeatConfig
	var createdAt int64
	var lastHeartbeat sql.NullInt64
	var template sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, chat_id, interval_mins, template, active_hours, timezone, enabled, last_heartbeat, created_at
		FROM heartbeat_configs WHERE chat_id = ?
	`, chatID).Scan(&config.ID, &config.ChatID, &config.IntervalMins, &template, &config.ActiveHours, &config.Timezone, &config.Enabled, &lastHeartbeat, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get heartbeat: %w", err)
	}
	config.CreatedAt = time.Unix(createdAt, 0)
	if lastHeartbeat.Valid {
		config.LastHeartbeat = time.Unix(lastHeartbeat.Int64, 0)
	}
	if template.Valid {
		config.Template = template.String
	}
	return &config, nil
}

func (r *memoryRepo) ListHeartbeats(ctx context.Context, enabledOnly bool) ([]*domain.HeartbeatConfig, error) {
	var rows *sql.Rows
	var err error
	if enabledOnly {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, chat_id, interval_mins, template, active_hours, timezone, enabled, last_heartbeat, created_at
			FROM heartbeat_configs WHERE enabled = 1
		`)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, chat_id, interval_mins, template, active_hours, timezone, enabled, last_heartbeat, created_at
			FROM heartbeat_configs
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list heartbeats: %w", err)
	}
	defer rows.Close()

	var configs []*domain.HeartbeatConfig
	for rows.Next() {
		var config domain.HeartbeatConfig
		var createdAt int64
		var lastHeartbeat sql.NullInt64
		var template sql.NullString
		if err := rows.Scan(&config.ID, &config.ChatID, &config.IntervalMins, &template, &config.ActiveHours, &config.Timezone, &config.Enabled, &lastHeartbeat, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan heartbeat: %w", err)
		}
		config.CreatedAt = time.Unix(createdAt, 0)
		if lastHeartbeat.Valid {
			config.LastHeartbeat = time.Unix(lastHeartbeat.Int64, 0)
		}
		if template.Valid {
			config.Template = template.String
		}
		configs = append(configs, &config)
	}
	return configs, nil
}

func (r *memoryRepo) UpdateHeartbeatTime(ctx context.Context, chatID string, lastHeartbeat time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE heartbeat_configs SET last_heartbeat = ? WHERE chat_id = ?
	`, lastHeartbeat.Unix(), chatID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat time: %w", err)
	}
	return nil
}

func (r *memoryRepo) DeleteHeartbeat(ctx context.Context, chatID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM heartbeat_configs WHERE chat_id = ?`, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	return nil
}

// Close closes the database connection
func (r *memoryRepo) Close() error {
	return r.db.Close()
}
