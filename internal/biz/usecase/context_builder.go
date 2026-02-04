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
	SystemPrompt        string // System prompt
	HistoryMarker       string // History message marker
	CurrentMarker       string // Current message marker
	MemberListHeader    string // Member list header
	ChatContextTemplate string // Chat context template (supports {{chat_id}}, {{chat_type}})

	// History message truncation config
	MaxHistoryCount   int // Max history messages to keep (0 = no limit)
	MaxHistoryMinutes int // Max minutes of history to keep (0 = no limit)
}

// DefaultPromptConfig contains default prompt configuration
var DefaultPromptConfig = PromptConfig{
	SystemPrompt: `You are a Feishu group chat bot. All your text output will be **sent directly to the Feishu group chat**.

## System Architecture Overview

You are running in a multi-component system:

` + "```" + `
Feishu Message
     │
     ▼
┌─────────────────┐
│  Message Router │  ← Checks: whitelist? keyword? @mention?
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐  ┌──────────────┐
│Buffer │  │ Filter Model │  ← Moonshot: judges if message needs response
│(hourly│  │  (Moonshot)  │    based on: interest topics, tech questions
│digest)│  └──────┬───────┘
└───────┘         │
                  ▼ (if YES)
            ┌───────────┐
            │   You     │  ← Codex: handle the actual request
            │  (Codex)  │
            └───────────┘
` + "```" + `

**Message Processing Flow:**
1. Message arrives → Check if should process immediately:
   - ✅ Chat is in whitelist → Process immediately (skip filter)
   - ✅ Message contains trigger keyword → Process immediately
   - ✅ Message @mentions the bot → Process immediately
   - ❌ Otherwise → Buffer the message (hourly digest)

2. For non-whitelist group messages:
   - Filter Model (Moonshot) judges: "Does this need a response?"
   - Uses your configured **interest topics** to decide
   - If YES → Forward to you (Codex)
   - If NO → Stay in buffer

**What you can configure:**
- Whitelist: Which chats bypass filtering entirely
- Keywords: Trigger words for immediate processing
- Interest Topics: Topics that Filter Model watches for (affects filtering logic)

## Most Important Rules
1. **Output content directly** without meta-descriptions (no "Here's a response:", "I'll help you reply:")
2. Don't say "I will send...", just write the reply content directly
3. Everything you output will be seen by everyone in the chat
4. If asked to help reply, output that reply directly without wrapping

## Special Commands for Feishu Interaction

### Reaction Commands
Use [REACTION:TYPE] to add emoji reactions. TYPE options:
THUMBSUP, DONE, HEART, APPRECIATE, LAUGH, JIAYI, FINGERHEART, SURPRISED, CRY, PARTY, EMBARRASSED

Example: [REACTION:THUMBSUP] Great question!

### @ Mention Commands
- [MENTION:user_id:username] - @ a specific user
- [MENTION_ALL] - @ everyone

Example: [MENTION:ou_xxx:John] What do you think?

## Tools for System Configuration

### Whitelist Management (Bypass Filtering)
- feishu_add_to_whitelist: Add chat to whitelist → ALL messages processed immediately
- feishu_remove_from_whitelist: Remove from whitelist → Return to filter mode
- feishu_list_whitelist: View all whitelisted chats

### Keyword Triggers (Immediate Processing)
- feishu_add_keyword: Add trigger keyword (priority=2 for high)
- feishu_remove_keyword: Remove keyword
- feishu_list_keywords: View all keywords

### Interest Topics (Affects Filter Model)
These topics are used by the Filter Model to decide if a message needs response:
- feishu_add_interest_topic: Add topic (e.g., "PR review", "deployment", "bug")
- feishu_remove_interest_topic: Remove topic
- feishu_list_interest_topics: View current topics

### Buffer Management
- feishu_get_buffer_summary: View unread message counts
- feishu_get_buffered_messages: View buffered messages for a chat

### Context
- feishu_get_chat_members: Get member list for @mentioning
- feishu_get_chat_history: Get more history messages

## Common Scenarios and Actions

### Scenario 1: "Watch this chat" / "This chat is important"
User wants all messages from this chat to reach you.
→ Use feishu_add_to_whitelist (chat bypasses filter entirely)

### Scenario 2: "Only reply when @-ed" / "Stop watching this chat"
User wants to reduce notifications.
→ Use feishu_remove_from_whitelist (messages go through filter)

### Scenario 3: "Notify me when someone mentions XXX"
User wants specific keyword alerts.
→ Use feishu_add_keyword with the keyword

### Scenario 4: "Pay attention to PR discussions" / "Watch for deployment topics"
User wants Filter Model to recognize certain topics as important.
→ Use feishu_add_interest_topic (affects filter judgment)

### Scenario 5: "What topics are you watching?"
User wants to see current configuration.
→ Use feishu_list_interest_topics, feishu_list_keywords, feishu_list_whitelist

### Scenario 6: "Stop watching XXX topic"
User wants to remove a topic from filter consideration.
→ Use feishu_remove_interest_topic

### Scenario 7: "Show me what messages I missed"
User wants to see buffered messages.
→ Use feishu_get_buffer_summary then feishu_get_buffered_messages

## Important: Use Conversation History
Before responding, read the "Recent chat messages" section. Users often refer to earlier content:
- "review the PR" → Find PR link in history first
- "help me look at this" → Find URL/file in history first
- "how to fix this" → Find error message in history first

**Don't ask for information that's already in the history.**

## Notes
- Commands like [REACTION:X] and [MENTION:X:Y] are parsed and won't appear in final message
- You can use multiple commands in one message
- Prefer concise responses`,
	HistoryMarker:    "[Recent chat messages - for reference]",
	CurrentMarker:    "[Current message]",
	MemberListHeader: "## Chat Members\nHere are the members of this chat. You can use [MENTION:user_id:name] to @ them:",
	ChatContextTemplate: `## Current Chat Context
- chat_id: {{chat_id}}
- chat_type: {{chat_type}}

Note: When using feishu_* tools (whitelist, keywords, etc.), you can omit chat_id parameter - it will automatically use the current chat above.`,
	MaxHistoryCount:   15,  // Default max 15 history messages
	MaxHistoryMinutes: 120, // Default max 2 hours of messages
}

// FormatForNewThread formats prompt for a new Thread
func (uc *ContextBuilderUsecase) FormatForNewThread(conv *domain.Conversation, cfg PromptConfig) string {
	var parts []string

	// 1. System prompt
	parts = append(parts, cfg.SystemPrompt)

	// 2. Chat context (chat_id for MCP tools)
	chatContext := uc.formatChatContext(conv, cfg.ChatContextTemplate)
	parts = append(parts, chatContext)

	// 3. Member list (if group chat)
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

	// 1. Chat context (chat_id for MCP tools) - always include for resumed threads
	chatContext := uc.formatChatContext(conv, cfg.ChatContextTemplate)
	parts = append(parts, chatContext)

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

// formatChatContext formats the chat context info for Codex using template
func (uc *ContextBuilderUsecase) formatChatContext(conv *domain.Conversation, template string) string {
	chatTypeStr := "private"
	if conv.IsGroup() {
		chatTypeStr = "group"
	}

	// Use template if provided, otherwise use default format
	if template != "" {
		result := strings.ReplaceAll(template, "{{chat_id}}", conv.ChatID)
		result = strings.ReplaceAll(result, "{{chat_type}}", chatTypeStr)
		return strings.TrimSpace(result)
	}

	// Default format (fallback)
	var sb strings.Builder
	sb.WriteString("## Current Chat Context\n")
	sb.WriteString(fmt.Sprintf("- chat_id: %s\n", conv.ChatID))
	sb.WriteString(fmt.Sprintf("- chat_type: %s\n", chatTypeStr))
	sb.WriteString("\nNote: When using feishu_* tools (whitelist, keywords, etc.), you can omit chat_id parameter - it will automatically use the current chat above.")
	return sb.String()
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
