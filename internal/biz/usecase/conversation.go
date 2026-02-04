package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// ConversationUsecase handles conversation logic (aggregate)
type ConversationUsecase struct {
	sessionUC *SessionUsecase
	contextUC *ContextBuilderUsecase
	codexRepo repo.CodexRepo
	promptCfg PromptConfig
}

// NewConversationUsecase creates a new conversation usecase
func NewConversationUsecase(
	sessionUC *SessionUsecase,
	contextUC *ContextBuilderUsecase,
	codexRepo repo.CodexRepo,
	promptCfg PromptConfig,
) *ConversationUsecase {
	return &ConversationUsecase{
		sessionUC: sessionUC,
		contextUC: contextUC,
		codexRepo: codexRepo,
		promptCfg: promptCfg,
	}
}

// TriggerRequest represents a trigger request
type TriggerRequest struct {
	ChatID        string
	MsgID         string
	Content       string
	SenderID      string
	SenderName    string
	ChatType      domain.ChatType
	ImagePaths    []string
	MsgCreateTime int64 // Message creation time (milliseconds Unix timestamp from Feishu)
}

// TriggerResponse represents a trigger response
type TriggerResponse struct {
	ThreadID string
	TurnID   string
	IsNew    bool
}

// Trigger triggers a conversation (core method)
func (uc *ConversationUsecase) Trigger(ctx context.Context, req *TriggerRequest) (*TriggerResponse, error) {
	// 1. Resolve Thread
	decision, err := uc.sessionUC.ResolveThread(ctx, req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("resolve thread: %w", err)
	}

	// 2. Build current message
	// Use Feishu's message time instead of local time.Now()
	// This ensures timestamp consistency with API-returned history messages
	msgTime := time.Now()
	if req.MsgCreateTime > 0 {
		msgTime = time.UnixMilli(req.MsgCreateTime)
	}
	current := &domain.Message{
		ID:         req.MsgID,
		ChatID:     req.ChatID,
		Content:    req.Content,
		SenderID:   req.SenderID,
		SenderName: req.SenderName,
		CreateTime: msgTime,
	}

	// 3. Build conversation context from Feishu API
	historyLimit := 20
	if decision.IsNew {
		historyLimit = 30 // Get more history for new Thread
	}

	conv, err := uc.contextUC.BuildConversation(ctx, req.ChatID, req.ChatType, current, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("build conversation: %w", err)
	}

	// 4. Format Prompt
	var prompt string
	if decision.IsNew {
		prompt = uc.contextUC.FormatForNewThread(conv, uc.promptCfg)
		fmt.Printf("[ConvUC] New thread prompt (%d chars), history=%d msgs\n", len(prompt), len(conv.History))
	} else {
		// Use LastProcessedMsgID as primary anchor, LastMsgTime as fallback
		// This ensures accurate "where to continue" even if bridge restarts
		prompt = uc.contextUC.FormatForResumedThread(conv, decision.LastProcessedMsgID, decision.LastMsgTime, decision.LastReplyAt, uc.promptCfg)
		fmt.Printf("[ConvUC] Resumed thread prompt (%d chars), history=%d msgs, lastMsgID=%s, lastMsgTime=%v\n",
			len(prompt), len(conv.History), decision.LastProcessedMsgID, decision.LastMsgTime)
	}

	// Debug: output full prompt
	fmt.Printf("[ConvUC] ========== FULL PROMPT START ==========\n%s\n[ConvUC] ========== FULL PROMPT END ==========\n", prompt)

	// 5. Send to Codex
	turnID, err := uc.codexRepo.StartTurn(ctx, decision.ThreadID, prompt, req.ImagePaths)
	if err != nil {
		return nil, fmt.Errorf("start turn: %w", err)
	}

	// 6. Update LastProcessedMsgID and LastMsgTime to current message
	// Use message ID as primary anchor, timestamp as fallback
	// This ensures accurate "where to continue" regardless of bridge restart
	if err := uc.sessionUC.UpdateLastProcessedMsg(ctx, req.ChatID, req.MsgID, current.CreateTime); err != nil {
		fmt.Printf("[ConvUC] Warning: failed to update last processed msg: %v\n", err)
	}

	return &TriggerResponse{
		ThreadID: decision.ThreadID,
		TurnID:   turnID,
		IsNew:    decision.IsNew,
	}, nil
}

// OnReplyComplete callback when reply is complete
func (uc *ConversationUsecase) OnReplyComplete(ctx context.Context, chatID string) error {
	return uc.sessionUC.MarkReplied(ctx, chatID)
}

// Touch updates session active time
func (uc *ConversationUsecase) Touch(ctx context.Context, chatID string) error {
	return uc.sessionUC.Touch(ctx, chatID)
}
