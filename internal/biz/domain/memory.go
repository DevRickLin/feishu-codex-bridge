package domain

import "time"

// MemoryEntry represents a memory entry stored by the bot
type MemoryEntry struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`      // Memory key/title
	Content   string    `json:"content"`  // Memory content
	Category  string    `json:"category"` // Category: "fact", "preference", "reminder", "note"
	ChatID    string    `json:"chat_id"`  // Optional: associated chat
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ScheduledTask represents a scheduled/cron task
type ScheduledTask struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`           // Task name/description
	Prompt        string    `json:"prompt"`         // Prompt to send to Codex when task runs
	ScheduleType  string    `json:"schedule_type"`  // "cron", "interval", "once"
	ScheduleValue string    `json:"schedule_value"` // cron: "0 9 * * 1" | interval: "3600000" (ms) | once: ISO timestamp
	ChatID        string    `json:"chat_id"`        // Target chat to send result
	Enabled       bool      `json:"enabled"`
	NextRun       time.Time `json:"next_run"`
	LastRun       time.Time `json:"last_run"`
	LastStatus    string    `json:"last_status"` // "ok", "error", "pending"
	LastError     string    `json:"last_error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// HeartbeatConfig represents heartbeat configuration for a chat
type HeartbeatConfig struct {
	ID            int64     `json:"id"`
	ChatID        string    `json:"chat_id"`
	IntervalMins  int       `json:"interval_mins"` // Heartbeat interval in minutes
	Template      string    `json:"template"`      // Message template
	ActiveHours   string    `json:"active_hours"`  // e.g., "09:00-18:00"
	Timezone      string    `json:"timezone"`      // e.g., "Asia/Shanghai"
	Enabled       bool      `json:"enabled"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CreatedAt     time.Time `json:"created_at"`
}
