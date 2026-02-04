package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

type mockMessageRepo struct {
	history []domain.Message
	members []domain.Member
}

func (m *mockMessageRepo) GetChatHistory(ctx context.Context, chatID string, limit int) ([]domain.Message, error) {
	if limit > len(m.history) {
		limit = len(m.history)
	}
	return m.history[:limit], nil
}

func (m *mockMessageRepo) GetChatMembers(ctx context.Context, chatID string) ([]domain.Member, error) {
	return m.members, nil
}

func (m *mockMessageRepo) GetChatInfo(ctx context.Context, chatID string) (*repo.ChatInfo, error) {
	return &repo.ChatInfo{ChatID: chatID, ChatType: domain.ChatTypeGroup}, nil
}

func (m *mockMessageRepo) SendText(ctx context.Context, chatID, text string) error {
	return nil
}

func (m *mockMessageRepo) SendTextWithMentions(ctx context.Context, chatID, text string, mentions []domain.Member) error {
	return nil
}

func (m *mockMessageRepo) AddReaction(ctx context.Context, msgID, reactionType string) error {
	return nil
}

func TestBuildConversation(t *testing.T) {
	now := time.Now()
	msgRepo := &mockMessageRepo{
		history: []domain.Message{
			{ID: "1", Content: "Hello", SenderName: "Alice", CreateTime: now.Add(-10 * time.Minute)},
			{ID: "2", Content: "Hi there", SenderName: "Bob", CreateTime: now.Add(-5 * time.Minute)},
		},
		members: []domain.Member{
			{UserID: "u1", Name: "Alice"},
			{UserID: "u2", Name: "Bob"},
		},
	}

	uc := NewContextBuilderUsecase(msgRepo)

	currentMsg := &domain.Message{
		ID:         "3",
		Content:    "Current message",
		SenderName: "Alice",
		CreateTime: now,
	}

	conv, err := uc.BuildConversation(context.Background(), "chat-123", domain.ChatTypeGroup, currentMsg, 10)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if conv.ChatID != "chat-123" {
		t.Errorf("Expected chat-123, got %s", conv.ChatID)
	}
	if len(conv.History) != 2 {
		t.Errorf("Expected 2 history messages, got %d", len(conv.History))
	}
	if len(conv.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(conv.Members))
	}
	if conv.Current.Content != "Current message" {
		t.Errorf("Expected current message content, got %s", conv.Current.Content)
	}
}

func TestFormatForNewThread(t *testing.T) {
	uc := &ContextBuilderUsecase{}

	now := time.Now()
	conv := &domain.Conversation{
		ChatID:   "chat-123",
		ChatType: domain.ChatTypeGroup,
		Members: []domain.Member{
			{UserID: "u1", Name: "Alice"},
		},
		History: []domain.Message{
			{ID: "1", Content: "Hello", SenderName: "Alice", CreateTime: now.Add(-5 * time.Minute)},
		},
		Current: &domain.Message{
			ID:         "2",
			Content:    "Current",
			SenderName: "Bob",
			SenderID:   "u2",
			CreateTime: now,
		},
	}

	cfg := DefaultPromptConfig
	prompt := uc.FormatForNewThread(conv, cfg)

	// Should contain system prompt
	if !strings.Contains(prompt, "Feishu group chat bot") {
		t.Error("Expected prompt to contain system prompt")
	}
	// Should contain member list
	if !strings.Contains(prompt, "Alice") {
		t.Error("Expected prompt to contain member Alice")
	}
	// Should contain history
	if !strings.Contains(prompt, "Hello") {
		t.Error("Expected prompt to contain history message")
	}
	// Should contain current message
	if !strings.Contains(prompt, "Current") {
		t.Error("Expected prompt to contain current message")
	}
}

func TestFormatForResumedThread(t *testing.T) {
	uc := &ContextBuilderUsecase{}

	now := time.Now()
	lastMsgTime := now.Add(-10 * time.Minute)
	lastReplyAt := now.Add(-10 * time.Minute)

	conv := &domain.Conversation{
		ChatID:   "chat-123",
		ChatType: domain.ChatTypeGroup,
		History: []domain.Message{
			{ID: "1", Content: "Before reply", SenderName: "Alice", CreateTime: now.Add(-15 * time.Minute)},
			{ID: "2", Content: "After reply", SenderName: "Bob", CreateTime: now.Add(-5 * time.Minute)},
		},
		Current: &domain.Message{
			ID:         "3",
			Content:    "Current",
			SenderName: "Alice",
			CreateTime: now,
		},
	}

	cfg := DefaultPromptConfig
	// Pass empty lastProcessedMsgID, use lastMsgTime as fallback
	prompt := uc.FormatForResumedThread(conv, "", lastMsgTime, lastReplyAt, cfg)

	// Should contain message after lastMsgTime
	if !strings.Contains(prompt, "After reply") {
		t.Error("Expected prompt to contain 'After reply'")
	}
	// Should contain current message
	if !strings.Contains(prompt, "Current") {
		t.Error("Expected prompt to contain current message")
	}
	// Should NOT contain old message (before lastMsgTime)
	if strings.Contains(prompt, "Before reply") {
		t.Error("Did not expect prompt to contain 'Before reply'")
	}
}

func TestFormatForResumedThread_NoRecentHistory(t *testing.T) {
	uc := &ContextBuilderUsecase{}

	now := time.Now()
	lastMsgTime := now.Add(-5 * time.Minute)
	lastReplyAt := now.Add(-5 * time.Minute)

	conv := &domain.Conversation{
		ChatID:   "chat-123",
		ChatType: domain.ChatTypeGroup,
		History: []domain.Message{
			// All messages before lastMsgTime
			{ID: "1", Content: "Old message", SenderName: "Alice", CreateTime: now.Add(-15 * time.Minute)},
		},
		Current: &domain.Message{
			ID:         "2",
			Content:    "Current",
			SenderName: "Bob",
			CreateTime: now,
		},
	}

	cfg := DefaultPromptConfig
	// Pass empty lastProcessedMsgID, use lastMsgTime as fallback
	prompt := uc.FormatForResumedThread(conv, "", lastMsgTime, lastReplyAt, cfg)

	// Should only contain current message
	if !strings.Contains(prompt, "Current") {
		t.Error("Expected prompt to contain current message")
	}
	if strings.Contains(prompt, "Old message") {
		t.Error("Did not expect old messages")
	}
}

func TestFormatForResumedThread_WithDisconnection(t *testing.T) {
	// Test disconnection recovery scenario: lastMsgTime is before lastReplyAt
	uc := &ContextBuilderUsecase{}

	now := time.Now()
	// Scenario:
	// - lastReplyAt: bot replied 10 minutes ago
	// - lastMsgTime: last processed message was 15 minutes ago
	// - A message at 12 minutes ago (after lastReplyAt, but after lastMsgTime)
	lastMsgTime := now.Add(-15 * time.Minute)
	lastReplyAt := now.Add(-10 * time.Minute)

	conv := &domain.Conversation{
		ChatID:   "chat-123",
		ChatType: domain.ChatTypeGroup,
		History: []domain.Message{
			{ID: "1", Content: "Before lastMsgTime", SenderName: "Alice", CreateTime: now.Add(-20 * time.Minute)},
			{ID: "2", Content: "Disconnected message", SenderName: "Bob", CreateTime: now.Add(-12 * time.Minute)}, // Message during disconnection
			{ID: "3", Content: "After lastReplyAt", SenderName: "Alice", CreateTime: now.Add(-5 * time.Minute)},
		},
		Current: &domain.Message{
			ID:         "4",
			Content:    "Current",
			SenderName: "Bob",
			CreateTime: now,
		},
	}

	cfg := DefaultPromptConfig
	// Pass empty lastProcessedMsgID, use lastMsgTime as fallback
	prompt := uc.FormatForResumedThread(conv, "", lastMsgTime, lastReplyAt, cfg)

	// Should contain all messages after lastMsgTime, including those during disconnection
	if !strings.Contains(prompt, "Disconnected message") {
		t.Error("Expected prompt to contain 'Disconnected message' (from disconnection period)")
	}
	if !strings.Contains(prompt, "After lastReplyAt") {
		t.Error("Expected prompt to contain 'After lastReplyAt'")
	}
	if !strings.Contains(prompt, "Current") {
		t.Error("Expected prompt to contain current message")
	}
	// Should NOT contain messages before lastMsgTime
	if strings.Contains(prompt, "Before lastMsgTime") {
		t.Error("Did not expect prompt to contain 'Before lastMsgTime'")
	}
}

func TestFormatHistoryForFilter(t *testing.T) {
	uc := &ContextBuilderUsecase{}

	messages := []domain.Message{
		{SenderName: "Alice", Content: "Hello"},
		{SenderName: "Bob", Content: "Hi there"},
	}

	result := uc.FormatHistoryForFilter(messages)

	if !strings.Contains(result, "[Alice]: Hello") {
		t.Error("Expected formatted Alice message")
	}
	if !strings.Contains(result, "[Bob]: Hi there") {
		t.Error("Expected formatted Bob message")
	}
}

func TestTruncateHistory_ByCount(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	// Create 20 messages
	messages := make([]domain.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = domain.Message{
			ID:         string(rune('a' + i)),
			Content:    "Message " + string(rune('A'+i)),
			CreateTime: now.Add(-time.Duration(20-i) * time.Minute), // Old to new
		}
	}

	cfg := PromptConfig{
		MaxHistoryCount:   5,
		MaxHistoryMinutes: 0, // No time limit
	}

	result := uc.truncateHistory(messages, cfg)

	if len(result) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(result))
	}
	// Should be the last 5 (newest)
	if result[0].Content != "Message P" {
		t.Errorf("Expected first message to be 'Message P', got '%s'", result[0].Content)
	}
	if result[4].Content != "Message T" {
		t.Errorf("Expected last message to be 'Message T', got '%s'", result[4].Content)
	}
}

func TestTruncateHistory_ByTime(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	messages := []domain.Message{
		{ID: "1", Content: "Old", CreateTime: now.Add(-3 * time.Hour)},  // Outside time window
		{ID: "2", Content: "Old2", CreateTime: now.Add(-2 * time.Hour)}, // Outside time window
		{ID: "3", Content: "Recent1", CreateTime: now.Add(-30 * time.Minute)},
		{ID: "4", Content: "Recent2", CreateTime: now.Add(-10 * time.Minute)},
	}

	cfg := PromptConfig{
		MaxHistoryCount:   0,  // No count limit -> keep all as "recent N"
		MaxHistoryMinutes: 60, // Time window (but recent N are kept unconditionally)
	}

	result := uc.truncateHistory(messages, cfg)

	// MaxHistoryCount=0 means all messages are kept as "recent N" unconditionally
	if len(result) != 4 {
		t.Errorf("Expected 4 messages (all kept as recent), got %d", len(result))
	}
}

func TestTruncateHistory_Combined(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	// Create 10 messages, all within 1 hour
	// 55, 50, 45, 40, 35, 30, 25, 20, 15, 10 minutes ago
	messages := make([]domain.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = domain.Message{
			ID:         string(rune('a' + i)),
			Content:    "Msg" + string(rune('A'+i)),
			CreateTime: now.Add(-time.Duration(55-i*5) * time.Minute),
		}
	}

	cfg := PromptConfig{
		MaxHistoryCount:   3,  // Unconditionally keep recent 3
		MaxHistoryMinutes: 60, // Extra messages within time window
	}

	result := uc.truncateHistory(messages, cfg)

	// Recent 3 kept unconditionally (MsgH, MsgI, MsgJ - 20, 15, 10 minutes ago)
	// Extra: remaining 7, within 60 min window (MsgA-MsgG are 55-25 min ago, all in window)
	// Total should be 7 + 3 = 10
	if len(result) != 10 {
		t.Errorf("Expected 10 messages (3 recent + 7 in time window), got %d", len(result))
	}
}

func TestTruncateHistory_Combined_WithOldMessages(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	// Create messages: 5 outside time window, 5 within window
	messages := []domain.Message{
		{ID: "1", Content: "VeryOld1", CreateTime: now.Add(-5 * time.Hour)},
		{ID: "2", Content: "VeryOld2", CreateTime: now.Add(-4 * time.Hour)},
		{ID: "3", Content: "VeryOld3", CreateTime: now.Add(-3 * time.Hour)},
		{ID: "4", Content: "Old1", CreateTime: now.Add(-90 * time.Minute)}, // Outside 1 hour
		{ID: "5", Content: "Old2", CreateTime: now.Add(-70 * time.Minute)}, // Outside 1 hour
		{ID: "6", Content: "Recent1", CreateTime: now.Add(-50 * time.Minute)},
		{ID: "7", Content: "Recent2", CreateTime: now.Add(-30 * time.Minute)},
		{ID: "8", Content: "Recent3", CreateTime: now.Add(-20 * time.Minute)},
		{ID: "9", Content: "Recent4", CreateTime: now.Add(-10 * time.Minute)},
		{ID: "10", Content: "Recent5", CreateTime: now.Add(-5 * time.Minute)},
	}

	cfg := PromptConfig{
		MaxHistoryCount:   3,  // Unconditionally keep recent 3
		MaxHistoryMinutes: 60, // Time window
	}

	result := uc.truncateHistory(messages, cfg)

	// Recent 3 kept unconditionally: Recent3, Recent4, Recent5
	// Remaining 7, within 60 min: Recent1, Recent2 (50, 30 min ago)
	// Total should be 2 + 3 = 5
	if len(result) != 5 {
		t.Errorf("Expected 5 messages (3 recent + 2 in time window), got %d", len(result))
	}

	// Verify order: extra first, recent last
	if result[0].Content != "Recent1" {
		t.Errorf("Expected first message to be 'Recent1', got '%s'", result[0].Content)
	}
	if result[4].Content != "Recent5" {
		t.Errorf("Expected last message to be 'Recent5', got '%s'", result[4].Content)
	}
}

func TestTruncateHistory_NoLimit(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	messages := []domain.Message{
		{ID: "1", Content: "Msg1", CreateTime: now.Add(-10 * time.Minute)},
		{ID: "2", Content: "Msg2", CreateTime: now.Add(-5 * time.Minute)},
	}

	cfg := PromptConfig{
		MaxHistoryCount:   0, // No limit
		MaxHistoryMinutes: 0, // No limit
	}

	result := uc.truncateHistory(messages, cfg)

	if len(result) != 2 {
		t.Errorf("Expected all 2 messages, got %d", len(result))
	}
}

func TestFormatForNewThread_WithTruncatedSummary(t *testing.T) {
	uc := &ContextBuilderUsecase{}
	now := time.Now()

	// Create 20 messages, first 10 outside time window
	history := make([]domain.Message, 20)
	for i := 0; i < 10; i++ {
		// First 10: 3-4 hours ago (outside time window)
		history[i] = domain.Message{
			ID:         string(rune('a' + i)),
			Content:    "Very old message " + string(rune('A'+i)),
			SenderName: "User",
			CreateTime: now.Add(-time.Duration(240-i*10) * time.Minute),
		}
	}
	for i := 10; i < 20; i++ {
		// Last 10: 10-100 minutes ago
		history[i] = domain.Message{
			ID:         string(rune('a' + i)),
			Content:    "Recent message " + string(rune('A'+i)),
			SenderName: "User",
			CreateTime: now.Add(-time.Duration(100-(i-10)*10) * time.Minute),
		}
	}

	conv := &domain.Conversation{
		ChatID:   "chat-123",
		ChatType: domain.ChatTypeP2P,
		History:  history,
		Current: &domain.Message{
			ID:         "current",
			Content:    "Current message",
			SenderName: "Bob",
			CreateTime: now,
		},
	}

	cfg := PromptConfig{
		SystemPrompt:      "Test system prompt",
		HistoryMarker:     "[History]",
		CurrentMarker:     "[Current]",
		MaxHistoryCount:   5,   // Unconditionally keep recent 5
		MaxHistoryMinutes: 120, // Extra messages within 2 hours
	}

	prompt := uc.FormatForNewThread(conv, cfg)

	// Recent 5 kept unconditionally
	// Remaining 15, within 2 hours extra kept (first 5 of last 10)
	// First 10 (3-4 hours ago) should be truncated

	// Should contain truncation summary
	if !strings.Contains(prompt, "earlier messages omitted") {
		t.Error("Expected prompt to contain truncation summary")
	}
	if !strings.Contains(prompt, "feishu_get_chat_history") {
		t.Error("Expected prompt to mention feishu_get_chat_history tool")
	}
	// Should contain summary samples
	if !strings.Contains(prompt, "Summary") {
		t.Error("Expected prompt to contain summary samples")
	}
}
