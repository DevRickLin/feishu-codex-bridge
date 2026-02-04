package mcp

// ToolDefinition represents an MCP tool definition
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// GetToolDefinitions returns all available MCP tool definitions
func GetToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "feishu_get_chat_members",
			Description: "Get the list of members in the current group chat for @mentioning.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "feishu_get_chat_history",
			Description: "Get recent messages from the current chat for context.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of messages to retrieve (default 20)",
					},
				},
			},
		},
		// Whitelist management tools
		{
			Name:        "feishu_add_to_whitelist",
			Description: "Add a chat to the instant response whitelist. Messages from whitelisted chats will be processed immediately instead of being buffered. Use when user says 'watch this chat', 'this chat is important', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "The chat_id to add to whitelist. Use current chat if not specified.",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Reason for adding to whitelist",
					},
				},
			},
		},
		{
			Name:        "feishu_remove_from_whitelist",
			Description: "Remove a chat from the instant response whitelist. Use when user says 'stop watching this chat', 'this chat is not important anymore', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "The chat_id to remove from whitelist. Use current chat if not specified.",
					},
				},
			},
		},
		{
			Name:        "feishu_list_whitelist",
			Description: "List all chats in the instant response whitelist.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// Keyword management tools
		{
			Name:        "feishu_add_keyword",
			Description: "Add a trigger keyword. Messages containing this keyword will trigger immediate response. Use when user says 'notify me when you see XXX', 'watch for keyword XXX', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keyword": map[string]interface{}{
						"type":        "string",
						"description": "The keyword to trigger on",
					},
					"priority": map[string]interface{}{
						"type":        "integer",
						"description": "Priority level: 1=normal, 2=high (immediate response)",
						"default":     1,
					},
				},
				"required": []string{"keyword"},
			},
		},
		{
			Name:        "feishu_remove_keyword",
			Description: "Remove a trigger keyword.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keyword": map[string]interface{}{
						"type":        "string",
						"description": "The keyword to remove",
					},
				},
				"required": []string{"keyword"},
			},
		},
		{
			Name:        "feishu_list_keywords",
			Description: "List all trigger keywords.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// Buffer viewing tools
		{
			Name:        "feishu_get_buffer_summary",
			Description: "Get a summary of buffered messages. Shows how many unread messages are waiting in each chat.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "feishu_get_buffered_messages",
			Description: "Get buffered messages from a specific chat.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "The chat_id to get messages from. Use current chat if not specified.",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of messages to retrieve (default 50)",
					},
				},
			},
		},
		// Interest topic management tools
		{
			Name:        "feishu_add_interest_topic",
			Description: "Add a topic that the bot should pay attention to. Messages about these topics will be processed even without @mention. Use when user says 'pay attention to discussions about XXX', 'I'm interested in XXX topic', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"topic": map[string]interface{}{
						"type":        "string",
						"description": "The topic to watch for (e.g., 'PR review', 'deployment', 'bug')",
					},
				},
				"required": []string{"topic"},
			},
		},
		{
			Name:        "feishu_remove_interest_topic",
			Description: "Remove a topic from the watch list. Use when user says 'stop watching XXX', 'I don't care about XXX anymore', or 'only respond when @-ed'.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"topic": map[string]interface{}{
						"type":        "string",
						"description": "The topic to remove",
					},
				},
				"required": []string{"topic"},
			},
		},
		{
			Name:        "feishu_list_interest_topics",
			Description: "List all topics the bot is currently watching.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}
