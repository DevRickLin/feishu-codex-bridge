package service

import (
	"context"
	"fmt"
	"strings"
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

// Default heartbeat prompt - instructs the agent what to do during heartbeat
const defaultHeartbeatPrompt = `[Heartbeat Check-in]

This is an automated heartbeat. Check if there's anything that needs attention:
1. Review any pending tasks or reminders
2. Check if there are buffered messages that need response
3. If nothing needs attention, reply with just: HEARTBEAT_OK

If you have something to report or need to alert the user, respond normally.
Do not repeat old information or infer tasks from prior conversations.

Chat ID: %s`

// runHeartbeat runs a single heartbeat by invoking the Codex agent
func (r *CronRunner) runHeartbeat(ctx context.Context, config *domain.HeartbeatConfig) {
	fmt.Printf("[CronRunner] Running heartbeat for chat: %s\n", config.ChatID)

	startTime := time.Now()

	// Create a new thread for this heartbeat
	threadID, err := r.codexRepo.CreateThread(ctx)
	if err != nil {
		fmt.Printf("[CronRunner] Error creating thread for heartbeat %s: %v\n", config.ChatID, err)
		return
	}

	// Build heartbeat prompt
	prompt := config.Template
	if prompt == "" {
		prompt = fmt.Sprintf(defaultHeartbeatPrompt, config.ChatID)
	} else {
		// User provided a custom template, wrap it with context
		prompt = fmt.Sprintf(`[Heartbeat Check-in]

%s

If nothing needs attention, reply with just: HEARTBEAT_OK

Chat ID: %s`, prompt, config.ChatID)
	}

	// Start turn with the heartbeat prompt
	_, err = r.codexRepo.StartTurn(ctx, threadID, prompt, nil)
	if err != nil {
		fmt.Printf("[CronRunner] Error starting turn for heartbeat %s: %v\n", config.ChatID, err)
		return
	}

	// Collect response from Codex
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
			fmt.Printf("[CronRunner] Error in heartbeat %s: %s\n", config.ChatID, errMsg)
			return
		}
	}

	// Update last heartbeat time
	r.memoryUC.UpdateHeartbeatTime(ctx, config.ChatID)

	// Check if response is just HEARTBEAT_OK (nothing to report)
	trimmedResponse := strings.TrimSpace(response)
	if isHeartbeatOK(trimmedResponse) {
		fmt.Printf("[CronRunner] Heartbeat OK for chat %s (no alert)\n", config.ChatID)
		return
	}

	// Strip HEARTBEAT_OK token if present alongside other content
	cleanResponse := stripHeartbeatToken(trimmedResponse)

	// Send the agent's response to chat if there's meaningful content
	if cleanResponse != "" && config.ChatID != "" {
		err = r.messageRepo.SendText(ctx, config.ChatID, cleanResponse)
		if err != nil {
			fmt.Printf("[CronRunner] Error sending heartbeat response to chat %s: %v\n", config.ChatID, err)
			return
		}
	}

	duration := time.Since(startTime)
	fmt.Printf("[CronRunner] Heartbeat for chat %s completed in %v\n", config.ChatID, duration)
}

// isHeartbeatOK checks if the response indicates nothing needs attention
func isHeartbeatOK(response string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(response))
	// Check for exact match or common variations
	okTokens := []string{"HEARTBEAT_OK", "HEARTBEAT OK", "HEARTBEATOOK"}
	for _, token := range okTokens {
		if normalized == token {
			return true
		}
	}
	// Also check if response is very short and contains the token
	if len(response) < 50 && strings.Contains(normalized, "HEARTBEAT_OK") {
		return true
	}
	return false
}

// stripHeartbeatToken removes HEARTBEAT_OK token from response
func stripHeartbeatToken(response string) string {
	// Remove the token from start and end
	result := response
	tokens := []string{"HEARTBEAT_OK", "HEARTBEAT OK", "**HEARTBEAT_OK**", "*HEARTBEAT_OK*"}
	for _, token := range tokens {
		result = strings.TrimPrefix(result, token)
		result = strings.TrimSuffix(result, token)
	}
	return strings.TrimSpace(result)
}
