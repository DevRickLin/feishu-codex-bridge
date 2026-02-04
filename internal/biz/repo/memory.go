package repo

import (
	"context"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
)

// MemoryRepo defines the memory storage interface
type MemoryRepo interface {
	// Memory operations
	SaveMemory(ctx context.Context, entry *domain.MemoryEntry) error
	GetMemory(ctx context.Context, key string) (*domain.MemoryEntry, error)
	SearchMemory(ctx context.Context, query string, limit int) ([]*domain.MemoryEntry, error)
	ListMemories(ctx context.Context, category string, limit int) ([]*domain.MemoryEntry, error)
	DeleteMemory(ctx context.Context, key string) error

	// Scheduled task operations
	CreateTask(ctx context.Context, task *domain.ScheduledTask) error
	GetTask(ctx context.Context, id int64) (*domain.ScheduledTask, error)
	GetTaskByName(ctx context.Context, name string) (*domain.ScheduledTask, error)
	ListTasks(ctx context.Context, enabledOnly bool) ([]*domain.ScheduledTask, error)
	GetDueTasks(ctx context.Context, now time.Time) ([]*domain.ScheduledTask, error)
	UpdateTaskAfterRun(ctx context.Context, id int64, nextRun time.Time, status, errorMsg string) error
	EnableTask(ctx context.Context, id int64, enabled bool) error
	DeleteTask(ctx context.Context, id int64) error

	// Heartbeat operations
	SetHeartbeat(ctx context.Context, config *domain.HeartbeatConfig) error
	GetHeartbeat(ctx context.Context, chatID string) (*domain.HeartbeatConfig, error)
	ListHeartbeats(ctx context.Context, enabledOnly bool) ([]*domain.HeartbeatConfig, error)
	UpdateHeartbeatTime(ctx context.Context, chatID string, lastHeartbeat time.Time) error
	DeleteHeartbeat(ctx context.Context, chatID string) error
}
