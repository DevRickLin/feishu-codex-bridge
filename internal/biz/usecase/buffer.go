package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// BufferConfig contains buffer configuration
type BufferConfig struct {
	DigestInterval time.Duration // Digest interval, default 1 hour
	MaxBufferAge   time.Duration // Max buffer time, force process after this
	CleanupAge     time.Duration // Time to cleanup processed messages
}

// DefaultBufferConfig returns default buffer configuration
func DefaultBufferConfig() BufferConfig {
	return BufferConfig{
		DigestInterval: 1 * time.Hour,
		MaxBufferAge:   2 * time.Hour,
		CleanupAge:     24 * time.Hour,
	}
}

// BufferUsecase handles message buffering logic
type BufferUsecase struct {
	bufferRepo repo.BufferRepo
	config     BufferConfig
}

// NewBufferUsecase creates a new buffer usecase
func NewBufferUsecase(bufferRepo repo.BufferRepo, config BufferConfig) *BufferUsecase {
	return &BufferUsecase{
		bufferRepo: bufferRepo,
		config:     config,
	}
}

// ShouldProcessImmediately determines if a message should be processed immediately
// Returns true for immediate processing, false for buffering
func (uc *BufferUsecase) ShouldProcessImmediately(ctx context.Context, chatID, content string, mentionsBot bool) (bool, string) {
	// 1. Bot mentioned -> process immediately
	if mentionsBot {
		return true, "mentioned"
	}

	// 2. Whitelisted chat -> process immediately
	inWhitelist, err := uc.bufferRepo.IsInWhitelist(ctx, chatID)
	if err == nil && inWhitelist {
		return true, "whitelist"
	}

	// 3. High priority keyword match -> process immediately
	kw, err := uc.bufferRepo.MatchKeyword(ctx, content)
	if err == nil && kw != nil && kw.Priority >= 2 {
		return true, fmt.Sprintf("keyword:%s", kw.Keyword)
	}

	// Otherwise add to buffer
	return false, ""
}

// AddToBuffer adds a message to the buffer
func (uc *BufferUsecase) AddToBuffer(ctx context.Context, msg *domain.BufferedMessage) error {
	return uc.bufferRepo.AddMessage(ctx, msg)
}

// GetUnprocessedMessages gets unprocessed messages for a specific chat
func (uc *BufferUsecase) GetUnprocessedMessages(ctx context.Context, chatID string) ([]*domain.BufferedMessage, error) {
	return uc.bufferRepo.GetUnprocessedMessages(ctx, chatID)
}

// GetAllUnprocessedMessages gets all unprocessed messages
func (uc *BufferUsecase) GetAllUnprocessedMessages(ctx context.Context) ([]*domain.BufferedMessage, error) {
	return uc.bufferRepo.GetAllUnprocessedMessages(ctx)
}

// MarkProcessed marks messages as processed
func (uc *BufferUsecase) MarkProcessed(ctx context.Context, msgIDs []int64) error {
	return uc.bufferRepo.MarkProcessed(ctx, msgIDs)
}

// GetBufferSummary gets buffer summary
func (uc *BufferUsecase) GetBufferSummary(ctx context.Context) ([]*domain.BufferSummary, error) {
	return uc.bufferRepo.GetBufferSummary(ctx)
}

// ========== Whitelist Management ==========

// AddToWhitelist adds to whitelist
func (uc *BufferUsecase) AddToWhitelist(ctx context.Context, chatID, reason, addedBy string) error {
	entry := &domain.WhitelistEntry{
		ChatID:  chatID,
		Reason:  reason,
		AddedBy: addedBy,
	}
	return uc.bufferRepo.AddToWhitelist(ctx, entry)
}

// RemoveFromWhitelist removes from whitelist
func (uc *BufferUsecase) RemoveFromWhitelist(ctx context.Context, chatID string) error {
	return uc.bufferRepo.RemoveFromWhitelist(ctx, chatID)
}

// GetWhitelist gets the whitelist
func (uc *BufferUsecase) GetWhitelist(ctx context.Context) ([]*domain.WhitelistEntry, error) {
	return uc.bufferRepo.GetWhitelist(ctx)
}

// ========== Keyword Management ==========

// AddKeyword adds a keyword
func (uc *BufferUsecase) AddKeyword(ctx context.Context, keyword string, priority int) error {
	kw := &domain.TriggerKeyword{
		Keyword:  keyword,
		Priority: priority,
	}
	return uc.bufferRepo.AddKeyword(ctx, kw)
}

// RemoveKeyword removes a keyword
func (uc *BufferUsecase) RemoveKeyword(ctx context.Context, keyword string) error {
	return uc.bufferRepo.RemoveKeyword(ctx, keyword)
}

// GetKeywords gets keyword list
func (uc *BufferUsecase) GetKeywords(ctx context.Context) ([]*domain.TriggerKeyword, error) {
	return uc.bufferRepo.GetKeywords(ctx)
}

// ========== Interest Topic Management (for Moonshot filtering) ==========

// AddInterestTopic adds an interest topic
func (uc *BufferUsecase) AddInterestTopic(ctx context.Context, topic, description string) error {
	return uc.bufferRepo.AddInterestTopic(ctx, topic, description)
}

// RemoveInterestTopic removes an interest topic
func (uc *BufferUsecase) RemoveInterestTopic(ctx context.Context, topic string) error {
	return uc.bufferRepo.RemoveInterestTopic(ctx, topic)
}

// GetInterestTopics gets interest topic list
func (uc *BufferUsecase) GetInterestTopics(ctx context.Context) ([]string, error) {
	return uc.bufferRepo.GetInterestTopics(ctx)
}

// ========== Scheduled Tasks ==========

// Cleanup cleans up expired data
func (uc *BufferUsecase) Cleanup(ctx context.Context) (int64, error) {
	before := time.Now().Add(-uc.config.CleanupAge)
	return uc.bufferRepo.CleanupOld(ctx, before)
}

// GetMessagesForDigest gets messages for digest (grouped by chat)
func (uc *BufferUsecase) GetMessagesForDigest(ctx context.Context) (map[string][]*domain.BufferedMessage, error) {
	messages, err := uc.bufferRepo.GetAllUnprocessedMessages(ctx)
	if err != nil {
		return nil, err
	}

	// Group by chatID
	grouped := make(map[string][]*domain.BufferedMessage)
	for _, msg := range messages {
		grouped[msg.ChatID] = append(grouped[msg.ChatID], msg)
	}

	return grouped, nil
}
