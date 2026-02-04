package mcp

import (
	"encoding/json"
	"fmt"
)

// Handler handles MCP tool calls using the HTTP client
type Handler struct {
	client *Client
}

// NewHandler creates a new MCP handler
func NewHandler(client *Client) *Handler {
	return &Handler{client: client}
}

// HandleToolCall handles a tool call and returns the result
func (h *Handler) HandleToolCall(name string, args map[string]interface{}) (interface{}, error) {
	// Get current context for default values
	ctx, err := h.client.GetContext()
	if err != nil {
		// Context fetch failed, use empty context
		ctx = &ChatContext{}
	}

	switch name {
	case "feishu_get_chat_members":
		return h.handleGetChatMembers(ctx, args)
	case "feishu_get_chat_history":
		return h.handleGetChatHistory(ctx, args)
	case "feishu_add_to_whitelist":
		return h.handleAddToWhitelist(ctx, args)
	case "feishu_remove_from_whitelist":
		return h.handleRemoveFromWhitelist(ctx, args)
	case "feishu_list_whitelist":
		return h.handleListWhitelist(ctx, args)
	case "feishu_add_keyword":
		return h.handleAddKeyword(ctx, args)
	case "feishu_remove_keyword":
		return h.handleRemoveKeyword(ctx, args)
	case "feishu_list_keywords":
		return h.handleListKeywords(ctx, args)
	case "feishu_get_buffer_summary":
		return h.handleGetBufferSummary(ctx, args)
	case "feishu_get_buffered_messages":
		return h.handleGetBufferedMessages(ctx, args)
	case "feishu_add_interest_topic":
		return h.handleAddInterestTopic(ctx, args)
	case "feishu_remove_interest_topic":
		return h.handleRemoveInterestTopic(ctx, args)
	case "feishu_list_interest_topics":
		return h.handleListInterestTopics(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ============ Chat Handlers ============

func (h *Handler) handleGetChatMembers(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID := getStringArg(args, "chat_id", ctx.ChatID)
	if chatID == "" {
		return map[string]interface{}{
			"members": []Member{},
			"note":    "No chat context available",
		}, nil
	}

	members, err := h.client.GetChatMembers(chatID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"members": members}, nil
}

func (h *Handler) handleGetChatHistory(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID := getStringArg(args, "chat_id", ctx.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("no chat context available")
	}

	limit := getIntArg(args, "limit", 20)
	messages, err := h.client.GetChatHistory(chatID, limit)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"messages": messages,
		"source":   "feishu_api",
	}, nil
}

// ============ Whitelist Handlers ============

func (h *Handler) handleAddToWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID := getStringArg(args, "chat_id", ctx.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	reason := getStringArg(args, "reason", "Added by Codex")

	if err := h.client.AddToWhitelist(chatID, reason); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Chat %s added to whitelist", chatID),
	}, nil
}

func (h *Handler) handleRemoveFromWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID := getStringArg(args, "chat_id", ctx.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	if err := h.client.RemoveFromWhitelist(chatID); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Chat %s removed from whitelist", chatID),
	}, nil
}

func (h *Handler) handleListWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	entries, err := h.client.GetWhitelist()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"entries": entries}, nil
}

// ============ Keyword Handlers ============

func (h *Handler) handleAddKeyword(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	keyword := getStringArg(args, "keyword", "")
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	priority := getIntArg(args, "priority", 1)

	if err := h.client.AddKeyword(keyword, priority); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Keyword '%s' added with priority %d", keyword, priority),
	}, nil
}

func (h *Handler) handleRemoveKeyword(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	keyword := getStringArg(args, "keyword", "")
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	if err := h.client.RemoveKeyword(keyword); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Keyword '%s' removed", keyword),
	}, nil
}

func (h *Handler) handleListKeywords(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	keywords, err := h.client.GetKeywords()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"keywords": keywords}, nil
}

// ============ Buffer Handlers ============

func (h *Handler) handleGetBufferSummary(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	summaries, err := h.client.GetBufferSummary()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"summaries": summaries}, nil
}

func (h *Handler) handleGetBufferedMessages(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID := getStringArg(args, "chat_id", ctx.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	limit := getIntArg(args, "limit", 50)

	messages, err := h.client.GetBufferedMessages(chatID, limit)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"messages": messages}, nil
}

// ============ Topic Handlers ============

func (h *Handler) handleAddInterestTopic(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	topic := getStringArg(args, "topic", "")
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	if err := h.client.AddTopic(topic); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Now watching for topic: %s", topic),
	}, nil
}

func (h *Handler) handleRemoveInterestTopic(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	topic := getStringArg(args, "topic", "")
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	if err := h.client.RemoveTopic(topic); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Stopped watching topic: %s", topic),
	}, nil
}

func (h *Handler) handleListInterestTopics(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	topics, err := h.client.GetTopics()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"topics": topics}, nil
}

// ============ Helpers ============

func getStringArg(args map[string]interface{}, key, defaultValue string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return defaultValue
}

func getIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	if v, ok := args[key].(int); ok {
		return v
	}
	return defaultValue
}

// FormatToolResult formats a tool result for MCP response
func FormatToolResult(result interface{}, isError bool) map[string]interface{} {
	content := ""
	if result != nil {
		if jsonBytes, err := json.Marshal(result); err == nil {
			content = string(jsonBytes)
		} else {
			content = fmt.Sprintf("%v", result)
		}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
		"isError": isError,
	}
}
