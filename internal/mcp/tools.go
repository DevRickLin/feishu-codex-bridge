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
		// ============ Memory Management Tools ============
		{
			Name:        "feishu_save_memory",
			Description: "Save a memory entry for later retrieval. Use for remembering facts, user preferences, important information, or anything you want to recall later. The key should be descriptive (e.g., 'user_preference_language', 'project_deadline', 'meeting_notes_2024_01_15').",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Unique key for this memory (use snake_case, descriptive names)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "The content to remember",
					},
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Category for organizing memories (e.g., 'preference', 'fact', 'note', 'task'). Default: 'note'",
					},
				},
				"required": []string{"key", "content"},
			},
		},
		{
			Name:        "feishu_get_memory",
			Description: "Retrieve a specific memory by its key.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The key of the memory to retrieve",
					},
				},
				"required": []string{"key"},
			},
		},
		{
			Name:        "feishu_search_memory",
			Description: "Search memories by content. Returns memories that match the search query using full-text search.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query to find relevant memories",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default 10)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "feishu_list_memories",
			Description: "List memories by category. Useful for browsing memories of a specific type.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Category to filter by (leave empty for all categories)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default 20)",
					},
				},
			},
		},
		{
			Name:        "feishu_delete_memory",
			Description: "Delete a memory entry by its key.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The key of the memory to delete",
					},
				},
				"required": []string{"key"},
			},
		},
		// ============ Scheduled Task Tools ============
		{
			Name:        "feishu_schedule_task",
			Description: "Create a scheduled task that will run at specified times. Use for reminders, periodic reports, or any recurring action. Schedule types: 'once' (ISO timestamp), 'interval' (milliseconds), 'cron' (cron expression like '0 9 * * *' for daily at 9am).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Unique name for this task",
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "The prompt/message to execute when the task runs",
					},
					"schedule_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of schedule: 'once', 'interval', or 'cron'",
						"enum":        []string{"once", "interval", "cron"},
					},
					"schedule_value": map[string]interface{}{
						"type":        "string",
						"description": "Schedule value: ISO timestamp for 'once', milliseconds for 'interval', cron expression for 'cron'",
					},
				},
				"required": []string{"name", "prompt", "schedule_type", "schedule_value"},
			},
		},
		{
			Name:        "feishu_list_tasks",
			Description: "List all scheduled tasks.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"enabled_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Only show enabled tasks (default true)",
					},
				},
			},
		},
		{
			Name:        "feishu_delete_task",
			Description: "Delete a scheduled task by name.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the task to delete",
					},
				},
				"required": []string{"name"},
			},
		},
		// ============ Heartbeat Tools ============
		{
			Name:        "feishu_set_heartbeat",
			Description: "Set up periodic heartbeat messages for a chat. Heartbeats are proactive check-ins that run during specified hours. Use when user says 'check in every hour', 'send daily updates', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interval_mins": map[string]interface{}{
						"type":        "integer",
						"description": "Interval between heartbeats in minutes (default 30)",
					},
					"template": map[string]interface{}{
						"type":        "string",
						"description": "Message template for heartbeat (optional, system will generate if not provided)",
					},
					"active_hours": map[string]interface{}{
						"type":        "string",
						"description": "Active hours range like '09:00-18:00' (default '00:00-23:59' for 24/7)",
					},
					"timezone": map[string]interface{}{
						"type":        "string",
						"description": "Timezone for active hours (default 'Asia/Shanghai')",
					},
				},
			},
		},
		{
			Name:        "feishu_list_heartbeats",
			Description: "List all heartbeat configurations.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"enabled_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Only show enabled heartbeats (default true)",
					},
				},
			},
		},
		{
			Name:        "feishu_delete_heartbeat",
			Description: "Delete heartbeat configuration for the current chat.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}
