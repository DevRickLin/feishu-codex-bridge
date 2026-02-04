package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// ConversationService handles conversation logic
type ConversationService struct {
	convUC      *usecase.ConversationUsecase
	filterUC    *usecase.FilterUsecase
	messageRepo repo.MessageRepo
	codexRepo   repo.CodexRepo

	// Chat states
	chatStates map[string]*ChatState
	statesMu   sync.RWMutex

	// Callback
	onReply func(chatID, msgID, text string, mentions []domain.Member)
}

// ChatState represents the state of a chat
type ChatState struct {
	mu         sync.Mutex
	ThreadID   string
	TurnID     string
	MsgID      string
	Processing bool
	Buffer     strings.Builder
}

// NewConversationService creates a new conversation service
func NewConversationService(
	convUC *usecase.ConversationUsecase,
	filterUC *usecase.FilterUsecase,
	messageRepo repo.MessageRepo,
	codexRepo repo.CodexRepo,
) *ConversationService {
	return &ConversationService{
		convUC:      convUC,
		filterUC:    filterUC,
		messageRepo: messageRepo,
		codexRepo:   codexRepo,
		chatStates:  make(map[string]*ChatState),
	}
}

// SetReplyCallback sets the reply callback
func (s *ConversationService) SetReplyCallback(callback func(chatID, msgID, text string, mentions []domain.Member)) {
	s.onReply = callback
}

// MessageRequest represents a message request
type MessageRequest struct {
	ChatID        string
	MsgID         string
	Content       string
	SenderID      string
	SenderName    string
	ChatType      domain.ChatType
	MentionsBot   bool
	ImagePaths    []string
	MsgCreateTime int64 // Message creation time (milliseconds Unix timestamp from Feishu)
}

// HandleMessage processes a message
func (s *ConversationService) HandleMessage(ctx context.Context, req *MessageRequest) error {
	// 1. Get or create chat state
	state := s.getChatState(req.ChatID)

	// 2. Check if filtering is needed (group chat without @mention)
	if req.ChatType == domain.ChatTypeGroup && !req.MentionsBot {
		if s.filterUC.IsFilterEnabled() {
			should, err := s.filterUC.ShouldRespond(ctx, req.ChatID, req.Content, "")
			if err != nil {
				fmt.Printf("[Service] Filter error: %v\n", err)
			}
			if !should {
				fmt.Printf("[Service] Skipping irrelevant message\n")
				return nil
			}
		} else {
			// No filter configured, skip non-@ messages
			fmt.Printf("[Service] No filter, skipping non-@ group message\n")
			return nil
		}
	}

	// 3. Check if already processing
	state.mu.Lock()
	if state.Processing {
		state.mu.Unlock()
		return fmt.Errorf("already processing")
	}
	state.Processing = true
	state.MsgID = req.MsgID
	state.Buffer.Reset()
	state.mu.Unlock()

	// 4. Add processing reaction
	_ = s.messageRepo.AddReaction(ctx, req.MsgID, "OnIt")

	// 5. Trigger conversation
	go s.processMessage(ctx, req, state)

	return nil
}

func (s *ConversationService) processMessage(ctx context.Context, req *MessageRequest, state *ChatState) {
	defer func() {
		state.mu.Lock()
		state.Processing = false
		state.mu.Unlock()
	}()

	triggerReq := &usecase.TriggerRequest{
		ChatID:        req.ChatID,
		MsgID:         req.MsgID,
		Content:       req.Content,
		SenderID:      req.SenderID,
		SenderName:    req.SenderName,
		ChatType:      req.ChatType,
		ImagePaths:    req.ImagePaths,
		MsgCreateTime: req.MsgCreateTime,
	}

	resp, err := s.convUC.Trigger(ctx, triggerReq)
	if err != nil {
		fmt.Printf("[Service] Trigger error: %v\n", err)
		_ = s.messageRepo.SendText(ctx, req.ChatID, fmt.Sprintf("Error processing: %v", err))
		return
	}

	state.mu.Lock()
	state.ThreadID = resp.ThreadID
	state.TurnID = resp.TurnID
	state.mu.Unlock()

	fmt.Printf("[Service] Started turn %s in thread %s (isNew=%v)\n", resp.TurnID, resp.ThreadID, resp.IsNew)
}

// HandleCodexEvent handles Codex events
func (s *ConversationService) HandleCodexEvent(event repo.Event) {
	switch event.Type {
	case repo.EventTypeAgentDelta:
		if data, ok := event.Data.(*repo.AgentDeltaData); ok {
			s.handleAgentDelta(event.ThreadID, data.Delta)
		}

	case repo.EventTypeTurnComplete:
		s.handleTurnComplete(event.ThreadID)

	case repo.EventTypeError:
		if data, ok := event.Data.(*repo.ErrorData); ok {
			fmt.Printf("[Service] Codex error: %v\n", data.Error)
		}
	}
}

func (s *ConversationService) handleAgentDelta(threadID, delta string) {
	chatID := s.findChatByThread(threadID)
	if chatID == "" {
		return
	}

	state := s.getChatState(chatID)
	state.mu.Lock()
	state.Buffer.WriteString(delta)
	state.mu.Unlock()
}

func (s *ConversationService) handleTurnComplete(threadID string) {
	chatID := s.findChatByThread(threadID)
	if chatID == "" {
		return
	}

	state := s.getChatState(chatID)
	state.mu.Lock()
	response := state.Buffer.String()
	msgID := state.MsgID
	state.mu.Unlock()

	if response == "" {
		return
	}

	// Parse response
	text, mentions := s.parseResponse(response)

	// Add completion reaction
	ctx := context.Background()
	_ = s.messageRepo.AddReaction(ctx, msgID, "DONE")

	// Send reply
	if s.onReply != nil {
		s.onReply(chatID, msgID, text, mentions)
	}

	// Mark as replied
	_ = s.convUC.OnReplyComplete(ctx, chatID)

	fmt.Printf("[Service] Turn completed, sent %d chars to %s\n", len(text), chatID)
}

// parseResponse parses the response, extracting text and directives
func (s *ConversationService) parseResponse(response string) (string, []domain.Member) {
	text := response
	var mentions []domain.Member

	// Extract REACTION directives (match any non-] character for error tolerance)
	reactionRegex := regexp.MustCompile(`\[REACTION:[^\]]+\]`)
	text = reactionRegex.ReplaceAllString(text, "")

	// Extract MENTION directives
	mentionRegex := regexp.MustCompile(`\[MENTION:([^:]+):([^\]]+)\]`)
	mentionMatches := mentionRegex.FindAllStringSubmatch(text, -1)
	for _, match := range mentionMatches {
		if len(match) == 3 {
			mentions = append(mentions, domain.Member{
				UserID: match[1],
				Name:   match[2],
			})
		}
	}
	text = mentionRegex.ReplaceAllString(text, "")

	// Extract MENTION_ALL directives
	mentionAllRegex := regexp.MustCompile(`\[MENTION_ALL\]`)
	text = mentionAllRegex.ReplaceAllString(text, "")

	// Clean up extra whitespace
	text = strings.TrimSpace(text)

	return text, mentions
}

func (s *ConversationService) getChatState(chatID string) *ChatState {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	state, ok := s.chatStates[chatID]
	if !ok {
		state = &ChatState{}
		s.chatStates[chatID] = state
	}
	return state
}

func (s *ConversationService) findChatByThread(threadID string) string {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	for chatID, state := range s.chatStates {
		state.mu.Lock()
		tid := state.ThreadID
		state.mu.Unlock()
		if tid == threadID {
			return chatID
		}
	}
	return ""
}

// StartEventLoop starts the event loop
func (s *ConversationService) StartEventLoop() {
	go func() {
		for event := range s.codexRepo.Events() {
			s.HandleCodexEvent(event)
		}
	}()
}
