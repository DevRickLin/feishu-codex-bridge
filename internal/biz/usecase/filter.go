package usecase

import (
	"context"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// FilterUsecase handles filtering logic
type FilterUsecase struct {
	filterRepo  repo.FilterRepo
	messageRepo repo.MessageRepo
	contextUC   *ContextBuilderUsecase
}

// NewFilterUsecase creates a new filter usecase
func NewFilterUsecase(
	filterRepo repo.FilterRepo,
	messageRepo repo.MessageRepo,
	contextUC *ContextBuilderUsecase,
) *FilterUsecase {
	return &FilterUsecase{
		filterRepo:  filterRepo,
		messageRepo: messageRepo,
		contextUC:   contextUC,
	}
}

// ShouldRespond determines if the bot should respond
func (uc *FilterUsecase) ShouldRespond(
	ctx context.Context,
	chatID string,
	currentMessage string,
	strategy string,
) (bool, error) {
	// If no filter configured, respond by default
	if uc.filterRepo == nil {
		return true, nil
	}

	// Get recent history as context
	history, _ := uc.messageRepo.GetChatHistory(ctx, chatID, 10)

	// Format history
	historyText := uc.contextUC.FormatHistoryForFilter(history)

	// Call filter
	return uc.filterRepo.ShouldRespond(ctx, currentMessage, historyText, strategy)
}

// IsFilterEnabled returns whether filter is enabled
func (uc *FilterUsecase) IsFilterEnabled() bool {
	return uc.filterRepo != nil
}
