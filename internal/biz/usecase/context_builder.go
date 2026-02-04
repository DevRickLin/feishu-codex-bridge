package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// ContextBuilderUsecase handles context building logic
type ContextBuilderUsecase struct {
	messageRepo repo.MessageRepo
}

// NewContextBuilderUsecase creates a new context builder usecase
func NewContextBuilderUsecase(messageRepo repo.MessageRepo) *ContextBuilderUsecase {
	return &ContextBuilderUsecase{messageRepo: messageRepo}
}

// BuildConversation builds conversation context (from Feishu API)
func (uc *ContextBuilderUsecase) BuildConversation(
	ctx context.Context,
	chatID string,
	chatType domain.ChatType,
	current *domain.Message,
	historyLimit int,
) (*domain.Conversation, error) {
	// Get history from Feishu API
	history, err := uc.messageRepo.GetChatHistory(ctx, chatID, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("get chat history: %w", err)
	}

	// Get chat members
	var members []domain.Member
	if chatType == domain.ChatTypeGroup {
		members, _ = uc.messageRepo.GetChatMembers(ctx, chatID)
	}

	return &domain.Conversation{
		ChatID:   chatID,
		ChatType: chatType,
		Members:  members,
		History:  history,
		Current:  current,
	}, nil
}

// PromptConfig contains prompt configuration
type PromptConfig struct {
	SystemPrompt     string // System prompt
	HistoryMarker    string // History message marker
	CurrentMarker    string // Current message marker
	MemberListHeader string // Member list header

	// History message truncation config
	MaxHistoryCount   int // Max history messages to keep (0 = no limit)
	MaxHistoryMinutes int // Max minutes of history to keep (0 = no limit)
}

// DefaultPromptConfig contains default prompt configuration
var DefaultPromptConfig = PromptConfig{
	SystemPrompt: `You are a Feishu group chat bot. All your text output will be **sent directly to the Feishu group chat**.

## Most Important Rules
1. **Output content directly** without any meta-descriptions (like "Here's a response you can use:", "I'll help you reply:", "You could say:")
2. Don't say "I will send..." or "Here is the reply...", just write the reply content directly
3. Everything you output will be seen by everyone in the chat, so use a conversational tone
4. If a user asks you to help reply to a question, output that reply directly without wrapping

Wrong example: "Here's a response you can use: It depends on the scenario..."
Correct example: "It depends on the scenario..."

## Special Commands for Feishu Interaction

## Reaction Commands
Use [REACTION:TYPE] to add emoji reactions to user messages. TYPE must be one of (ALL CAPS, no spaces):
THUMBSUP, DONE, HEART, APPRECIATE, LAUGH, JIAYI, FINGERHEART, SURPRISED, CRY, PARTY, EMBARRASSED

Example: [REACTION:THUMBSUP] That's a great question!
Example: [REACTION:JIAYI] I agree with this point.

## @ Mention User Commands
Use [MENTION:user_id:username] to @ mention a specific user in your reply.
Use [MENTION_ALL] to @ mention everyone in the chat.

Examples:
[MENTION:ou_xxx:John] What do you think of this approach?
[MENTION_ALL] Attention everyone, this is an important announcement!

## Getting More Context
If you need to see earlier conversation history (e.g., when users refer to previously discussed content), use the feishu_get_chat_history tool to get more history messages.

## Message Notification Management
You can manage which chats need instant notifications. By default, non-@mentioned messages are buffered and summarized hourly.

You can use these tools to manage notification behavior:

### Whitelist Management
When user says "watch this chat", "this chat is important", "notify me in real-time for this chat", use:
- feishu_add_to_whitelist: Add current chat to whitelist, all messages will notify you instantly
- feishu_remove_from_whitelist: Remove from whitelist, return to buffer+summary mode
- feishu_list_whitelist: View all whitelisted chats

### Keyword Triggers
When user says "notify me when someone mentions XXX", "watch for keyword XXX", use:
- feishu_add_keyword: Add trigger keyword, messages containing it will notify instantly (priority=2 for high priority)
- feishu_remove_keyword: Remove keyword
- feishu_list_keywords: View all keywords

### View Buffered Messages
- feishu_get_buffer_summary: View unread message count overview for all chats
- feishu_get_buffered_messages: View buffered message details for a specific chat

### Interest Topic Management
You can set topics of interest, and the system will automatically watch for messages about these topics (even without @ mention):
- feishu_add_interest_topic: Add topic of interest (e.g., "PR review", "deployment", "bug")
- feishu_remove_interest_topic: Remove topic
- feishu_list_interest_topics: View currently watched topics

Example conversations:
User: "This chat is important, notify me when there are messages"
You: OK, I've added this chat to the whitelist. All messages here will now notify me instantly. [then call feishu_add_to_whitelist]

User: "Notify me when someone mentions bug or urgent"
You: OK, I've added keyword monitoring. [then call feishu_add_keyword twice for "bug" and "urgent"]

User: "Only reply when @-ed in this chat" or "Don't actively watch this chat anymore"
You: OK, I've removed this chat from the whitelist. I'll only reply when @-ed now. [then call feishu_remove_from_whitelist]

User: "Watch for PR-related discussions"
You: OK, I'll watch for PR-related discussions. [then call feishu_add_interest_topic with "PR"]

User: "Don't watch deployment anymore"
You: OK, I've stopped watching the deployment topic. [then call feishu_remove_interest_topic]

## Important: Use Conversation History
Before responding to user requests, carefully read the "Recent chat messages" section. Users often refer to earlier message content, for example:
- User says "review the PR" -> Find PR link or number in history first
- User says "help me look at this" -> Find related URL, file, or code in history first
- User says "how to fix this" -> Find problem description or error message in history first

**Don't ask users for information that's already clearly present in the history**. Use the information from history directly to start working.

Notes:
- Commands will be parsed and executed, and won't appear in the final message
- You can use multiple commands in a single message
- Prefer concise responses`,
	HistoryMarker:     "[Recent chat messages - for reference]",
	CurrentMarker:     "[Current message]",
	MemberListHeader:  "## Chat Members\nHere are the members of this chat. You can use [MENTION:user_id:name] to @ them:",
	MaxHistoryCount:   15,  // Default max 15 history messages
	MaxHistoryMinutes: 120, // Default max 2 hours of messages
}

// FormatForNewThread formats prompt for a new Thread
func (uc *ContextBuilderUsecase) FormatForNewThread(conv *domain.Conversation, cfg PromptConfig) string {
	var parts []string

	// 1. System prompt
	parts = append(parts, cfg.SystemPrompt)

	// 2. Member list (if group chat)
	if conv.IsGroup() && len(conv.Members) > 0 {
		memberList := uc.formatMemberList(conv.Members, cfg.MemberListHeader)
		parts = append(parts, memberList)
	}

	// 3. History messages (apply truncation strategy)
	fullHistory := conv.HistoryExcludingCurrent()
	truncatedHistory := uc.truncateHistory(fullHistory, cfg)

	// If messages were truncated, generate summary
	truncatedCount := len(fullHistory) - len(truncatedHistory)
	if truncatedCount > 0 {
		summary := uc.formatTruncatedSummary(fullHistory[:truncatedCount], truncatedCount)
		parts = append(parts, summary)
	}

	if len(truncatedHistory) > 0 {
		historyText := uc.formatHistory(truncatedHistory, cfg.HistoryMarker)
		parts = append(parts, historyText)
	}

	// 4. Current message
	if conv.Current != nil {
		currentText := uc.formatCurrentMessage(conv.Current, cfg.CurrentMarker)
		parts = append(parts, currentText)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// formatTruncatedSummary generates summary for truncated messages
func (uc *ContextBuilderUsecase) formatTruncatedSummary(truncatedMsgs []domain.Message, count int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%d earlier messages omitted. Use feishu_get_chat_history tool to view if needed]\n", count))

	// Take up to 3 messages as brief summary
	sampleCount := 3
	if len(truncatedMsgs) < sampleCount {
		sampleCount = len(truncatedMsgs)
	}

	if sampleCount > 0 {
		sb.WriteString("Summary:\n")
		// Take last few (closest to truncation point)
		startIdx := len(truncatedMsgs) - sampleCount
		for i := startIdx; i < len(truncatedMsgs); i++ {
			m := truncatedMsgs[i]
			name := m.SenderName
			if name == "" {
				name = m.SenderID
			}
			if m.IsBot {
				name = "You (bot)"
			}
			// Truncate content to max 50 chars
			content := m.Content
			if len(content) > 50 {
				content = content[:50] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - [%s]: %s\n", name, content))
		}
	}

	return sb.String()
}

// truncateHistory truncates history messages
// Strategy: unconditionally keep last N messages + additional messages within time window
func (uc *ContextBuilderUsecase) truncateHistory(messages []domain.Message, cfg PromptConfig) []domain.Message {
	if len(messages) == 0 {
		return messages
	}

	n := len(messages)

	// 1. Unconditionally keep last N messages
	recentCount := cfg.MaxHistoryCount
	if recentCount <= 0 || recentCount > n {
		recentCount = n
	}

	// Split: older messages | last N messages
	olderMessages := messages[:n-recentCount]
	recentMessages := messages[n-recentCount:]

	// 2. From older messages, filter by time window
	var extraMessages []domain.Message
	if cfg.MaxHistoryMinutes > 0 && len(olderMessages) > 0 {
		cutoffTime := time.Now().Add(-time.Duration(cfg.MaxHistoryMinutes) * time.Minute)
		for _, m := range olderMessages {
			if m.CreateTime.After(cutoffTime) {
				extraMessages = append(extraMessages, m)
			}
		}
	}

	// 3. Merge: additional time window messages + last N messages
	result := append(extraMessages, recentMessages...)

	return result
}

// FormatForResumedThread formats prompt for resumed Thread
// lastProcessedMsgID: Last processed message ID (primary anchor)
// lastMsgTime: Last processed message time (fallback anchor when msgID not found)
// lastReplyAt: Bot's last reply time (for reference only, not used)
func (uc *ContextBuilderUsecase) FormatForResumedThread(
	conv *domain.Conversation,
	lastProcessedMsgID string,
	lastMsgTime time.Time,
	lastReplyAt time.Time,
	cfg PromptConfig,
) string {
	var parts []string

	// Use message ID as primary anchor, lastMsgTime as fallback
	// This ensures accurate "where to continue" regardless of bridge restart
	recentHistory := conv.HistoryAfterMsgID(lastProcessedMsgID, lastMsgTime)

	if len(recentHistory) > 0 {
		historyText := uc.formatHistory(recentHistory, cfg.HistoryMarker)
		parts = append(parts, historyText)
	}

	// 2. Current message
	if conv.Current != nil {
		currentText := uc.formatCurrentMessage(conv.Current, cfg.CurrentMarker)
		parts = append(parts, currentText)
	}

	return strings.Join(parts, "\n\n")
}

func (uc *ContextBuilderUsecase) formatMemberList(members []domain.Member, header string) string {
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	for _, m := range members {
		sb.WriteString(fmt.Sprintf("- %s (user_id: %s)\n", m.Name, m.UserID))
	}
	return sb.String()
}

func (uc *ContextBuilderUsecase) formatHistory(messages []domain.Message, marker string) string {
	var sb strings.Builder
	sb.WriteString(marker)
	sb.WriteString("\n")
	for _, m := range messages {
		name := m.SenderName
		if name == "" {
			name = m.SenderID
		}
		if m.IsBot {
			// Mark as bot's (self) message so Codex knows this is its previous reply
			sb.WriteString(fmt.Sprintf("[You (bot)]: %s\n", m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", name, m.Content))
		}
	}
	return sb.String()
}

func (uc *ContextBuilderUsecase) formatCurrentMessage(msg *domain.Message, marker string) string {
	name := msg.SenderName
	if name == "" {
		name = msg.SenderID
	}
	if msg.SenderID != "" {
		return fmt.Sprintf("%s\n[Message from %s (user_id: %s)]:\n%s",
			marker, name, msg.SenderID, msg.Content)
	}
	return fmt.Sprintf("%s\n[Message from %s]:\n%s", marker, name, msg.Content)
}

// FormatHistoryForFilter formats history messages for filter
func (uc *ContextBuilderUsecase) FormatHistoryForFilter(messages []domain.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		name := m.SenderName
		if name == "" {
			name = m.SenderID
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", name, m.Content))
	}
	return sb.String()
}
