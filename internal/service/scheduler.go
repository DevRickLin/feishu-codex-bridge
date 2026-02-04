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

// DigestScheduler handles scheduled digest processing
type DigestScheduler struct {
	bufferUC  *usecase.BufferUsecase
	codexRepo repo.CodexRepo
	filterUC  *usecase.FilterUsecase // Moonshot filter
	convSvc   *ConversationService

	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewDigestScheduler creates a new digest scheduler
func NewDigestScheduler(
	bufferUC *usecase.BufferUsecase,
	codexRepo repo.CodexRepo,
	filterUC *usecase.FilterUsecase,
	convSvc *ConversationService,
	interval time.Duration,
) *DigestScheduler {
	return &DigestScheduler{
		bufferUC:  bufferUC,
		codexRepo: codexRepo,
		filterUC:  filterUC,
		convSvc:   convSvc,
		interval:  interval,
	}
}

// Start starts the scheduler
func (s *DigestScheduler) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(2)
	go s.digestLoop()
	go s.cleanupLoop()

	fmt.Printf("[Scheduler] Started with interval %v\n", s.interval)
}

// Stop stops the scheduler
func (s *DigestScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	fmt.Println("[Scheduler] Stopped")
}

// digestLoop is the digest processing loop
func (s *DigestScheduler) digestLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processDigests()
		}
	}
}

// cleanupLoop is the cleanup loop (runs every 6 hours)
func (s *DigestScheduler) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// processDigests processes all pending digests
func (s *DigestScheduler) processDigests() {
	ctx := context.Background()

	// Get messages grouped by chat
	grouped, err := s.bufferUC.GetMessagesForDigest(ctx)
	if err != nil {
		fmt.Printf("[Scheduler] Failed to get messages for digest: %v\n", err)
		return
	}

	if len(grouped) == 0 {
		fmt.Println("[Scheduler] No messages to digest")
		return
	}

	fmt.Printf("[Scheduler] Processing digests for %d chats\n", len(grouped))

	for chatID, messages := range grouped {
		if len(messages) == 0 {
			continue
		}

		// Must have Codex and ConversationService to process
		if s.codexRepo == nil || s.convSvc == nil {
			fmt.Printf("[Scheduler] Skipping %s: Codex or ConversationService not configured\n", chatID)
			continue
		}

		s.processWithCodex(ctx, chatID, messages)
	}
}

// processWithCodex processes buffered messages using Codex
func (s *DigestScheduler) processWithCodex(ctx context.Context, chatID string, messages []*domain.BufferedMessage) {
	// 1. First use Moonshot filter to check if these messages need a response
	if s.filterUC != nil && s.filterUC.IsFilterEnabled() {
		// Build history text for filtering
		historyText := s.buildHistoryForFilter(messages)
		// Use the last message as current message
		lastMsg := messages[len(messages)-1]
		currentContent := lastMsg.Content

		should, err := s.filterUC.ShouldRespond(ctx, chatID, currentContent, historyText)
		if err != nil {
			fmt.Printf("[Scheduler] Moonshot filter error for %s: %v, proceeding anyway\n", chatID, err)
		} else if !should {
			// Moonshot determined no response needed, just mark as processed
			fmt.Printf("[Scheduler] Moonshot: skip digest for %s (%d messages, not relevant)\n", chatID, len(messages))
			s.markMessagesProcessed(ctx, messages)
			return
		}
		fmt.Printf("[Scheduler] Moonshot: digest for %s needs response\n", chatID)
	}

	// 2. Build digest prompt
	prompt := s.buildDigestPrompt(messages)

	// Get last message info for tracking
	lastMsg := messages[len(messages)-1]

	// 3. Process through ConversationService (reuse existing message handling flow)
	req := &MessageRequest{
		ChatID:     chatID,
		MsgID:      fmt.Sprintf("digest_%d", time.Now().Unix()), // Virtual message ID
		Content:    prompt,
		SenderID:   "system",
		SenderName: "System Digest",
		ChatType:   domain.ChatTypeGroup,
	}

	if err := s.convSvc.HandleMessage(ctx, req); err != nil {
		fmt.Printf("[Scheduler] Failed to process digest with Codex for %s: %v\n", chatID, err)
		// On failure, just mark as processed, don't send any message to chat
		s.markMessagesProcessed(ctx, messages)
		return
	}

	// 4. Mark messages as processed
	s.markMessagesProcessed(ctx, messages)

	fmt.Printf("[Scheduler] Sent digest to Codex for %s (%d messages, last sender: %s)\n",
		chatID, len(messages), lastMsg.SenderName)
}

// buildHistoryForFilter builds history text for Moonshot filtering
func (s *DigestScheduler) buildHistoryForFilter(messages []*domain.BufferedMessage) string {
	var sb strings.Builder
	for _, msg := range messages {
		sender := msg.SenderName
		if sender == "" {
			sender = msg.SenderID
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", sender, msg.Content))
	}
	return sb.String()
}

// markMessagesProcessed marks messages as processed
func (s *DigestScheduler) markMessagesProcessed(ctx context.Context, messages []*domain.BufferedMessage) {
	var msgIDs []int64
	for _, msg := range messages {
		msgIDs = append(msgIDs, msg.ID)
	}
	if err := s.bufferUC.MarkProcessed(ctx, msgIDs); err != nil {
		fmt.Printf("[Scheduler] Failed to mark messages processed: %v\n", err)
	}
}

// buildDigestPrompt builds the digest prompt
func (s *DigestScheduler) buildDigestPrompt(messages []*domain.BufferedMessage) string {
	var sb strings.Builder

	sb.WriteString("[Scheduled Digest Task]\n\n")
	sb.WriteString("Below are the recent unread messages in this chat. Please provide a brief summary, ")
	sb.WriteString("and if there are questions or topics that need my response, please respond directly:\n\n")

	for _, msg := range messages {
		sender := msg.SenderName
		if sender == "" {
			sender = msg.SenderID
		}
		// Format time
		msgTime := time.Unix(msg.CreatedAt.Unix(), 0).Format("15:04")
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", msgTime, sender, msg.Content))
	}

	sb.WriteString("\n---\n")
	sb.WriteString("Based on the messages above:\n")
	sb.WriteString("1. If someone is asking a question or needs help, respond directly\n")
	sb.WriteString("2. If it's just casual chat, provide a brief summary, no need to reply\n")
	sb.WriteString("3. If the messages are not relevant to you, you may choose not to reply (output empty content)\n")

	return sb.String()
}

// cleanup cleans up expired messages
func (s *DigestScheduler) cleanup() {
	ctx := context.Background()

	count, err := s.bufferUC.Cleanup(ctx)
	if err != nil {
		fmt.Printf("[Scheduler] Cleanup error: %v\n", err)
		return
	}

	if count > 0 {
		fmt.Printf("[Scheduler] Cleaned up %d old messages\n", count)
	}
}
