package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// CronRunner runs scheduled tasks and heartbeats
type CronRunner struct {
	memoryUC    *usecase.MemoryUsecase
	messageRepo repo.MessageRepo
	codexRepo   repo.CodexRepo

	pollInterval time.Duration
	running      bool
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewCronRunner creates a new cron runner
func NewCronRunner(memoryUC *usecase.MemoryUsecase, messageRepo repo.MessageRepo, codexRepo repo.CodexRepo) *CronRunner {
	return &CronRunner{
		memoryUC:     memoryUC,
		messageRepo:  messageRepo,
		codexRepo:    codexRepo,
		pollInterval: 60 * time.Second, // Check every 60 seconds
		stopCh:       make(chan struct{}),
	}
}

// Start starts the cron runner
func (r *CronRunner) Start() {
	if r.running {
		return
	}
	r.running = true
	r.wg.Add(1)
	go r.loop()
	fmt.Printf("[CronRunner] Started with poll interval %v\n", r.pollInterval)
}

// Stop stops the cron runner
func (r *CronRunner) Stop() {
	if !r.running {
		return
	}
	r.running = false
	close(r.stopCh)
	r.wg.Wait()
	fmt.Println("[CronRunner] Stopped")
}

func (r *CronRunner) loop() {
	defer r.wg.Done()

	// Initial run
	r.runDueTasks()
	r.runDueHeartbeats()

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.runDueTasks()
			r.runDueHeartbeats()
		case <-r.stopCh:
			return
		}
	}
}

// runDueTasks runs all due scheduled tasks
func (r *CronRunner) runDueTasks() {
	ctx := context.Background()

	tasks, err := r.memoryUC.GetDueTasks(ctx)
	if err != nil {
		fmt.Printf("[CronRunner] Error getting due tasks: %v\n", err)
		return
	}

	for _, task := range tasks {
		r.runTask(ctx, task)
	}
}

// runTask runs a single scheduled task
func (r *CronRunner) runTask(ctx context.Context, task *domain.ScheduledTask) {
	fmt.Printf("[CronRunner] Running task: %s\n", task.Name)

	startTime := time.Now()

	// Create a new thread for this task
	threadID, err := r.codexRepo.CreateThread(ctx)
	if err != nil {
		r.memoryUC.UpdateTaskAfterRun(ctx, task, "error", "failed to create thread: "+err.Error())
		fmt.Printf("[CronRunner] Error creating thread for task %s: %v\n", task.Name, err)
		return
	}

	// Build prompt with context
	prompt := fmt.Sprintf(`[Scheduled Task: %s]

This is an automated scheduled task. Execute the following and send the result to the chat.

Task: %s

Chat ID: %s
`, task.Name, task.Prompt, task.ChatID)

	// Start turn
	_, err = r.codexRepo.StartTurn(ctx, threadID, prompt, nil)
	if err != nil {
		r.memoryUC.UpdateTaskAfterRun(ctx, task, "error", "failed to start turn: "+err.Error())
		fmt.Printf("[CronRunner] Error starting turn for task %s: %v\n", task.Name, err)
		return
	}

	// Collect response
	response := ""
	for event := range r.codexRepo.Events() {
		switch event.Type {
		case repo.EventTypeAgentDelta:
			if data, ok := event.Data.(*repo.AgentDeltaData); ok {
				response += data.Delta
			}
		case repo.EventTypeTurnComplete:
			// Turn is complete
		case repo.EventTypeError:
			errMsg := "unknown error"
			if data, ok := event.Data.(*repo.ErrorData); ok && data.Error != nil {
				errMsg = data.Error.Error()
			}
			r.memoryUC.UpdateTaskAfterRun(ctx, task, "error", errMsg)
			fmt.Printf("[CronRunner] Error in task %s: %s\n", task.Name, errMsg)
			return
		}
	}

	// Send response to chat if we have one
	if response != "" && task.ChatID != "" {
		err = r.messageRepo.SendText(ctx, task.ChatID, response)
		if err != nil {
			fmt.Printf("[CronRunner] Error sending task result to chat %s: %v\n", task.ChatID, err)
		}
	}

	duration := time.Since(startTime)
	r.memoryUC.UpdateTaskAfterRun(ctx, task, "ok", "")
	fmt.Printf("[CronRunner] Task %s completed in %v\n", task.Name, duration)
}

// runDueHeartbeats runs all due heartbeats
func (r *CronRunner) runDueHeartbeats() {
	ctx := context.Background()

	configs, err := r.memoryUC.GetDueHeartbeats(ctx)
	if err != nil {
		fmt.Printf("[CronRunner] Error getting due heartbeats: %v\n", err)
		return
	}

	for _, config := range configs {
		r.runHeartbeat(ctx, config)
	}
}

// runHeartbeat runs a single heartbeat
func (r *CronRunner) runHeartbeat(ctx context.Context, config *domain.HeartbeatConfig) {
	fmt.Printf("[CronRunner] Running heartbeat for chat: %s\n", config.ChatID)

	// Build heartbeat message
	message := config.Template
	if message == "" {
		message = "Hey! Just checking in. Is there anything you need help with? 👋"
	}

	// Send heartbeat message
	err := r.messageRepo.SendText(ctx, config.ChatID, message)
	if err != nil {
		fmt.Printf("[CronRunner] Error sending heartbeat to chat %s: %v\n", config.ChatID, err)
		return
	}

	// Update last heartbeat time
	r.memoryUC.UpdateHeartbeatTime(ctx, config.ChatID)
	fmt.Printf("[CronRunner] Heartbeat sent to chat: %s\n", config.ChatID)
}
