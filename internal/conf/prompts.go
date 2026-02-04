package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PromptsConfig contains all prompt configurations loaded from YAML
type PromptsConfig struct {
	Codex   CodexPrompts  `yaml:"codex"`
	Filter  FilterPrompts `yaml:"filter"`
	History HistoryConfig `yaml:"history"`
}

// CodexPrompts contains Codex-related prompts
type CodexPrompts struct {
	SystemPrompt        string `yaml:"system_prompt"`
	HistoryMarker       string `yaml:"history_marker"`
	CurrentMarker       string `yaml:"current_marker"`
	MemberListHeader    string `yaml:"member_list_header"`
	ChatContextTemplate string `yaml:"chat_context_template"`
}

// FilterPrompts contains filter model prompts
type FilterPrompts struct {
	StrategyTemplate      string `yaml:"strategy_template"`
	TopicsSectionTemplate string `yaml:"topics_section_template"`
	DefaultStrategy       string `yaml:"default_strategy"`
	SummaryPrompt         string `yaml:"summary_prompt"`
}

// HistoryConfig contains history truncation settings
type HistoryConfig struct {
	MaxCount   int `yaml:"max_count"`
	MaxMinutes int `yaml:"max_minutes"`
}

// LoadPromptsConfig loads prompts configuration from YAML file
func LoadPromptsConfig(configPath string) (*PromptsConfig, error) {
	// Try multiple paths
	paths := []string{configPath}
	if configPath == "" {
		paths = []string{
			"configs/prompts.yaml",
			"./configs/prompts.yaml",
			"/etc/feishu-codex-bridge/prompts.yaml",
		}
		// Add path relative to executable
		if execPath, err := os.Executable(); err == nil {
			paths = append(paths, filepath.Join(filepath.Dir(execPath), "configs", "prompts.yaml"))
		}
		// Add path relative to working directory
		if wd, err := os.Getwd(); err == nil {
			paths = append(paths, filepath.Join(wd, "configs", "prompts.yaml"))
		}
	}

	var data []byte
	var loadedPath string
	var err error

	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			loadedPath = p
			break
		}
	}

	if data == nil {
		// Return default config if no file found
		fmt.Println("[Config] No prompts.yaml found, using defaults")
		return DefaultPromptsConfig(), nil
	}

	fmt.Printf("[Config] Loading prompts from: %s\n", loadedPath)

	var config PromptsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse prompts.yaml: %w", err)
	}

	// Fill in defaults for empty values
	config.fillDefaults()

	return &config, nil
}

// fillDefaults fills in default values for empty fields
func (c *PromptsConfig) fillDefaults() {
	defaults := DefaultPromptsConfig()

	if c.Codex.SystemPrompt == "" {
		c.Codex.SystemPrompt = defaults.Codex.SystemPrompt
	}
	if c.Codex.HistoryMarker == "" {
		c.Codex.HistoryMarker = defaults.Codex.HistoryMarker
	}
	if c.Codex.CurrentMarker == "" {
		c.Codex.CurrentMarker = defaults.Codex.CurrentMarker
	}
	if c.Codex.MemberListHeader == "" {
		c.Codex.MemberListHeader = defaults.Codex.MemberListHeader
	}
	if c.Codex.ChatContextTemplate == "" {
		c.Codex.ChatContextTemplate = defaults.Codex.ChatContextTemplate
	}

	if c.Filter.StrategyTemplate == "" {
		c.Filter.StrategyTemplate = defaults.Filter.StrategyTemplate
	}
	if c.Filter.TopicsSectionTemplate == "" {
		c.Filter.TopicsSectionTemplate = defaults.Filter.TopicsSectionTemplate
	}
	if c.Filter.DefaultStrategy == "" {
		c.Filter.DefaultStrategy = defaults.Filter.DefaultStrategy
	}
	if c.Filter.SummaryPrompt == "" {
		c.Filter.SummaryPrompt = defaults.Filter.SummaryPrompt
	}

	if c.History.MaxCount == 0 {
		c.History.MaxCount = defaults.History.MaxCount
	}
	if c.History.MaxMinutes == 0 {
		c.History.MaxMinutes = defaults.History.MaxMinutes
	}
}

// GetFilterStrategy returns the filter strategy prompt with bot name and topics
func (c *PromptsConfig) GetFilterStrategy(botName string, topics []string) string {
	if botName == "" {
		return c.Filter.DefaultStrategy
	}

	strategy := c.Filter.StrategyTemplate

	// Build topics section
	topicsSection := ""
	if len(topics) > 0 {
		topicsSection = strings.ReplaceAll(c.Filter.TopicsSectionTemplate, "{{topics}}", strings.Join(topics, ", "))
	}

	// Replace placeholders
	strategy = strings.ReplaceAll(strategy, "{{bot_name}}", botName)
	strategy = strings.ReplaceAll(strategy, "{{topics_section}}", topicsSection)

	return strategy
}

// FormatChatContext formats the chat context with actual values
func (c *PromptsConfig) FormatChatContext(chatID, chatType string) string {
	result := c.Codex.ChatContextTemplate
	result = strings.ReplaceAll(result, "{{chat_id}}", chatID)
	result = strings.ReplaceAll(result, "{{chat_type}}", chatType)
	return strings.TrimSpace(result)
}

// DefaultPromptsConfig returns the default prompts configuration
func DefaultPromptsConfig() *PromptsConfig {
	return &PromptsConfig{
		Codex: CodexPrompts{
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
		},
		Filter: FilterPrompts{
			StrategyTemplate: `You are a message filter that determines whether group chat messages need a response from the bot "{{bot_name}}".

## Bot Information
- Name: {{bot_name}}
- Role: Programming assistant, skilled at code, technical questions, and file operations{{topics_section}}

## Recognizing @ Mentions
Messages can contain @ mentions in two formats:
1. @{{bot_name}} (using the bot name directly) -> Clearly calling the bot
2. @_user_1, @_user_2 placeholders -> System format, usually **NOT** the bot

**Key**: @_user_N format mentions are usually @ other users in the group, not the bot.
Only explicit @{{bot_name}} means they are calling the bot.

## Decision Rules
1. Message explicitly contains @{{bot_name}} -> YES
2. Message contains @_user_N (placeholder format) -> This is @ other users -> NO
3. Message has no @, but is asking technical/programming questions -> YES
4. Message is casual chat, unrelated to tech -> NO
5. Uncertain -> NO

Reply only YES or NO.`,
			TopicsSectionTemplate: "\n- Topics of interest: {{topics}} (if related to these topics -> YES)",
			DefaultStrategy: `You are a message filter that determines whether group chat messages need a response from the bot.

The bot is a programming assistant, skilled at code, technical questions, file operations, etc.

Decision rules:
1. If the message is asking technical/programming questions -> YES
2. If the message is casual chat, unrelated to the bot -> NO
3. If the message is explicitly calling/asking the bot -> YES
4. If uncertain -> NO

Reply only "YES" or "NO", no explanations.`,
			SummaryPrompt: `You are a conversation summarizer. Please summarize the chat history into a brief context description.

Requirements:
1. Keep key information: who said what important things, topics discussed, unresolved questions
2. Use third-person description
3. Keep it under 200 words
4. If the conversation is simple or has no important content, make it shorter
5. Output the summary directly, no prefix like "Summary:" needed`,
		},
		History: HistoryConfig{
			MaxCount:   15,
			MaxMinutes: 120,
		},
	}
}
