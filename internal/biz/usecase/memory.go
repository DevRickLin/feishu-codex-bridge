package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// MemoryUsecase handles memory, scheduling, and heartbeat operations
type MemoryUsecase struct {
	memoryRepo repo.MemoryRepo
}

// NewMemoryUsecase creates a new memory usecase
func NewMemoryUsecase(memoryRepo repo.MemoryRepo) *MemoryUsecase {
	return &MemoryUsecase{memoryRepo: memoryRepo}
}

// ========== Memory Operations ==========

// SaveMemory saves a memory entry
func (uc *MemoryUsecase) SaveMemory(ctx context.Context, key, content, category, chatID string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if content == "" {
		return fmt.Errorf("content is required")
	}
	if category == "" {
		category = "note"
	}
	entry := &domain.MemoryEntry{
		Key:      key,
		Content:  content,
		Category: category,
		ChatID:   chatID,
	}
	return uc.memoryRepo.SaveMemory(ctx, entry)
}

// GetMemory gets a memory by key
func (uc *MemoryUsecase) GetMemory(ctx context.Context, key string) (*domain.MemoryEntry, error) {
	return uc.memoryRepo.GetMemory(ctx, key)
}

// SearchMemory searches memories
func (uc *MemoryUsecase) SearchMemory(ctx context.Context, query string, limit int) ([]*domain.MemoryEntry, error) {
	return uc.memoryRepo.SearchMemory(ctx, query, limit)
}

// ListMemories lists memories by category
func (uc *MemoryUsecase) ListMemories(ctx context.Context, category string, limit int) ([]*domain.MemoryEntry, error) {
	return uc.memoryRepo.ListMemories(ctx, category, limit)
}

// DeleteMemory deletes a memory
func (uc *MemoryUsecase) DeleteMemory(ctx context.Context, key string) error {
	return uc.memoryRepo.DeleteMemory(ctx, key)
}

// ========== Scheduled Task Operations ==========

// ScheduleTask creates or updates a scheduled task
func (uc *MemoryUsecase) ScheduleTask(ctx context.Context, name, prompt, scheduleType, scheduleValue, chatID string) error {
	if name == "" {
		return fmt.Errorf("task name is required")
	}
	if prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if chatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	// Validate and calculate next run
	nextRun, err := uc.calculateNextRun(scheduleType, scheduleValue, time.Now())
	if err != nil {
		return err
	}

	task := &domain.ScheduledTask{
		Name:          name,
		Prompt:        prompt,
		ScheduleType:  scheduleType,
		ScheduleValue: scheduleValue,
		ChatID:        chatID,
		Enabled:       true,
		NextRun:       nextRun,
	}
	return uc.memoryRepo.CreateTask(ctx, task)
}

// calculateNextRun calculates the next run time based on schedule type and value
func (uc *MemoryUsecase) calculateNextRun(scheduleType, scheduleValue string, from time.Time) (time.Time, error) {
	switch scheduleType {
	case "once":
		// Parse ISO timestamp
		t, err := time.Parse(time.RFC3339, scheduleValue)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid once schedule value, expected ISO timestamp: %w", err)
		}
		return t, nil

	case "interval":
		// Parse milliseconds
		ms, err := strconv.ParseInt(scheduleValue, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid interval schedule value, expected milliseconds: %w", err)
		}
		return from.Add(time.Duration(ms) * time.Millisecond), nil

	case "cron":
		// Parse cron expression and get next run
		// For now, support simple patterns like "0 9 * * *" (daily at 9am)
		nextRun, err := uc.parseCronNextRun(scheduleValue, from)
		if err != nil {
			return time.Time{}, err
		}
		return nextRun, nil

	default:
		return time.Time{}, fmt.Errorf("invalid schedule type: %s (must be 'once', 'interval', or 'cron')", scheduleType)
	}
}

// parseCronNextRun parses a cron expression and returns the next run time
// Supports: minute hour day-of-month month day-of-week
func (uc *MemoryUsecase) parseCronNextRun(expr string, from time.Time) (time.Time, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression: expected 5 fields (min hour dom month dow)")
	}

	// Simple implementation: just handle common patterns
	minute := parts[0]
	hour := parts[1]
	// dom := parts[2]  // day of month
	// month := parts[3]
	dow := parts[4] // day of week

	// Parse minute and hour
	var targetMin, targetHour int
	if minute != "*" {
		m, err := strconv.Atoi(minute)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid minute in cron: %s", minute)
		}
		targetMin = m
	}
	if hour != "*" {
		h, err := strconv.Atoi(hour)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid hour in cron: %s", hour)
		}
		targetHour = h
	}

	// Start from next minute
	next := from.Add(time.Minute).Truncate(time.Minute)

	// Find next matching time (max 7 days ahead)
	for i := 0; i < 7*24*60; i++ {
		if minute != "*" && next.Minute() != targetMin {
			next = next.Add(time.Minute)
			continue
		}
		if hour != "*" && next.Hour() != targetHour {
			next = next.Add(time.Minute)
			continue
		}
		if dow != "*" {
			targetDow, err := strconv.Atoi(dow)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid day of week in cron: %s", dow)
			}
			if int(next.Weekday()) != targetDow {
				next = next.Add(time.Minute)
				continue
			}
		}
		return next, nil
	}

	return time.Time{}, fmt.Errorf("could not find next run time within 7 days")
}

// GetTask gets a task by ID
func (uc *MemoryUsecase) GetTask(ctx context.Context, id int64) (*domain.ScheduledTask, error) {
	return uc.memoryRepo.GetTask(ctx, id)
}

// GetTaskByName gets a task by name
func (uc *MemoryUsecase) GetTaskByName(ctx context.Context, name string) (*domain.ScheduledTask, error) {
	return uc.memoryRepo.GetTaskByName(ctx, name)
}

// ListTasks lists all tasks
func (uc *MemoryUsecase) ListTasks(ctx context.Context, enabledOnly bool) ([]*domain.ScheduledTask, error) {
	return uc.memoryRepo.ListTasks(ctx, enabledOnly)
}

// GetDueTasks gets all tasks that are due to run
func (uc *MemoryUsecase) GetDueTasks(ctx context.Context) ([]*domain.ScheduledTask, error) {
	return uc.memoryRepo.GetDueTasks(ctx, time.Now())
}

// UpdateTaskAfterRun updates a task after it has run
func (uc *MemoryUsecase) UpdateTaskAfterRun(ctx context.Context, task *domain.ScheduledTask, status, errorMsg string) error {
	var nextRun time.Time
	if task.ScheduleType != "once" {
		var err error
		nextRun, err = uc.calculateNextRun(task.ScheduleType, task.ScheduleValue, time.Now())
		if err != nil {
			// If we can't calculate next run, disable the task
			return uc.memoryRepo.UpdateTaskAfterRun(ctx, task.ID, time.Time{}, "error", "failed to calculate next run: "+err.Error())
		}
	}
	return uc.memoryRepo.UpdateTaskAfterRun(ctx, task.ID, nextRun, status, errorMsg)
}

// EnableTask enables or disables a task
func (uc *MemoryUsecase) EnableTask(ctx context.Context, id int64, enabled bool) error {
	return uc.memoryRepo.EnableTask(ctx, id, enabled)
}

// DeleteTask deletes a task
func (uc *MemoryUsecase) DeleteTask(ctx context.Context, id int64) error {
	return uc.memoryRepo.DeleteTask(ctx, id)
}

// DeleteTaskByName deletes a task by name
func (uc *MemoryUsecase) DeleteTaskByName(ctx context.Context, name string) error {
	task, err := uc.memoryRepo.GetTaskByName(ctx, name)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", name)
	}
	return uc.memoryRepo.DeleteTask(ctx, task.ID)
}

// ========== Heartbeat Operations ==========

// SetHeartbeat sets a heartbeat configuration for a chat
func (uc *MemoryUsecase) SetHeartbeat(ctx context.Context, chatID string, intervalMins int, template, activeHours, timezone string) error {
	if chatID == "" {
		return fmt.Errorf("chat_id is required")
	}
	if intervalMins <= 0 {
		intervalMins = 30
	}
	if activeHours == "" {
		activeHours = "00:00-23:59"
	}
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}

	config := &domain.HeartbeatConfig{
		ChatID:       chatID,
		IntervalMins: intervalMins,
		Template:     template,
		ActiveHours:  activeHours,
		Timezone:     timezone,
		Enabled:      true,
	}
	return uc.memoryRepo.SetHeartbeat(ctx, config)
}

// GetHeartbeat gets heartbeat config for a chat
func (uc *MemoryUsecase) GetHeartbeat(ctx context.Context, chatID string) (*domain.HeartbeatConfig, error) {
	return uc.memoryRepo.GetHeartbeat(ctx, chatID)
}

// ListHeartbeats lists all heartbeat configs
func (uc *MemoryUsecase) ListHeartbeats(ctx context.Context, enabledOnly bool) ([]*domain.HeartbeatConfig, error) {
	return uc.memoryRepo.ListHeartbeats(ctx, enabledOnly)
}

// GetDueHeartbeats gets all heartbeats that are due to run
func (uc *MemoryUsecase) GetDueHeartbeats(ctx context.Context) ([]*domain.HeartbeatConfig, error) {
	configs, err := uc.memoryRepo.ListHeartbeats(ctx, true)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var dueConfigs []*domain.HeartbeatConfig
	for _, config := range configs {
		// Check if within active hours
		if !uc.isWithinActiveHours(config.ActiveHours, config.Timezone, now) {
			continue
		}

		// Check if enough time has passed since last heartbeat
		interval := time.Duration(config.IntervalMins) * time.Minute
		if config.LastHeartbeat.IsZero() || now.Sub(config.LastHeartbeat) >= interval {
			dueConfigs = append(dueConfigs, config)
		}
	}
	return dueConfigs, nil
}

// isWithinActiveHours checks if current time is within active hours
func (uc *MemoryUsecase) isWithinActiveHours(activeHours, timezone string, now time.Time) bool {
	// Parse active hours like "09:00-18:00"
	parts := strings.Split(activeHours, "-")
	if len(parts) != 2 {
		return true // Invalid format, allow all
	}

	// Load timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.Local
	}
	localNow := now.In(loc)

	// Parse start and end times
	startParts := strings.Split(parts[0], ":")
	endParts := strings.Split(parts[1], ":")
	if len(startParts) != 2 || len(endParts) != 2 {
		return true
	}

	startHour, _ := strconv.Atoi(startParts[0])
	startMin, _ := strconv.Atoi(startParts[1])
	endHour, _ := strconv.Atoi(endParts[0])
	endMin, _ := strconv.Atoi(endParts[1])

	startMins := startHour*60 + startMin
	endMins := endHour*60 + endMin
	currentMins := localNow.Hour()*60 + localNow.Minute()

	return currentMins >= startMins && currentMins <= endMins
}

// UpdateHeartbeatTime updates the last heartbeat time
func (uc *MemoryUsecase) UpdateHeartbeatTime(ctx context.Context, chatID string) error {
	return uc.memoryRepo.UpdateHeartbeatTime(ctx, chatID, time.Now())
}

// DisableHeartbeat disables heartbeat for a chat
func (uc *MemoryUsecase) DisableHeartbeat(ctx context.Context, chatID string) error {
	config, err := uc.memoryRepo.GetHeartbeat(ctx, chatID)
	if err != nil {
		return err
	}
	if config == nil {
		return fmt.Errorf("heartbeat not found for chat: %s", chatID)
	}
	config.Enabled = false
	return uc.memoryRepo.SetHeartbeat(ctx, config)
}

// DeleteHeartbeat deletes heartbeat config
func (uc *MemoryUsecase) DeleteHeartbeat(ctx context.Context, chatID string) error {
	return uc.memoryRepo.DeleteHeartbeat(ctx, chatID)
}
