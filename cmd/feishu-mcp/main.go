package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// This MCP server communicates with the parent Bridge process via a Unix socket or file-based IPC.
// It provides Feishu tools to Codex and relays tool calls to the Bridge.

// IPC file paths - these are set via environment variables
var (
	ipcRequestFile  = os.Getenv("FEISHU_MCP_REQUEST_FILE")
	ipcResponseFile = os.Getenv("FEISHU_MCP_RESPONSE_FILE")
	contextFile     = os.Getenv("FEISHU_MCP_CONTEXT_FILE")
)

// Context loaded from file
type ChatContext struct {
	ChatID     string          `json:"chat_id"`
	ChatType   string          `json:"chat_type"`
	MessageID  string          `json:"message_id"`
	Members    []Member        `json:"members"`
	RecentMsgs []RecentMessage `json:"recent_msgs"`
}

type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RecentMessage struct {
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// MCP Protocol types
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool definitions
// NOTE: Message-sending tools (feishu_send_message, feishu_mention_user, feishu_mention_all, feishu_add_reaction)
// were removed. Codex should output text directly with [REACTION:TYPE] and [MENTION:id:name] markers.
// Bridge will parse these and send to the correct chat, avoiding IPC issues.
var tools = []map[string]interface{}{
	{
		"name":        "feishu_get_chat_members",
		"description": "Get the list of members in the current group chat for @mentioning.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		"name":        "feishu_get_chat_history",
		"description": "Get recent messages from the current chat for context.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_add_to_whitelist",
		"description": "Add a chat to the instant response whitelist. Messages from whitelisted chats will be processed immediately instead of being buffered. Use when user says 'watch this chat', 'this chat is important', etc.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_remove_from_whitelist",
		"description": "Remove a chat from the instant response whitelist. Use when user says 'stop watching this chat', 'this chat is not important anymore', etc.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_list_whitelist",
		"description": "List all chats in the instant response whitelist.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	// Keyword management tools
	{
		"name":        "feishu_add_keyword",
		"description": "Add a trigger keyword. Messages containing this keyword will trigger immediate response. Use when user says 'notify me when you see XXX', 'watch for keyword XXX', etc.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_remove_keyword",
		"description": "Remove a trigger keyword.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_list_keywords",
		"description": "List all trigger keywords.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	// Buffer viewing tools
	{
		"name":        "feishu_get_buffer_summary",
		"description": "Get a summary of buffered messages. Shows how many unread messages are waiting in each chat.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		"name":        "feishu_get_buffered_messages",
		"description": "Get buffered messages from a specific chat.",
		"inputSchema": map[string]interface{}{
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
	// Interest topic management tools (affects Moonshot filtering)
	{
		"name":        "feishu_add_interest_topic",
		"description": "Add a topic that the bot should pay attention to. Messages about these topics will be processed even without @mention. Use when user says 'pay attention to discussions about XXX', 'I'm interested in XXX topic', etc.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_remove_interest_topic",
		"description": "Remove a topic from the watch list. Use when user says 'stop watching XXX', 'I don't care about XXX anymore', or 'only respond when @-ed'.",
		"inputSchema": map[string]interface{}{
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
		"name":        "feishu_list_interest_topics",
		"description": "List all topics the bot is currently watching.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Read from stdin, write to stdout (MCP stdio transport)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *MCPRequest) {
	switch req.Method {
	case "initialize":
		handleInitialize(req)
	case "tools/list":
		handleToolsList(req)
	case "tools/call":
		handleToolsCall(req)
	case "notifications/initialized":
		// Notification, no response needed
	default:
		sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func handleInitialize(req *MCPRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "feishu-mcp",
			"version": "1.0.0",
		},
	}
	sendResult(req.ID, result)
}

func handleToolsList(req *MCPRequest) {
	result := map[string]interface{}{
		"tools": tools,
	}
	sendResult(req.ID, result)
}

func handleToolsCall(req *MCPRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	// Load current context
	ctx := loadContext()

	var result interface{}
	var err error

	switch params.Name {
	case "feishu_get_chat_members":
		result, err = handleGetChatMembers(ctx, params.Arguments)
	case "feishu_get_chat_history":
		result, err = handleGetChatHistory(ctx, params.Arguments)
	// Whitelist management
	case "feishu_add_to_whitelist":
		result, err = handleAddToWhitelist(ctx, params.Arguments)
	case "feishu_remove_from_whitelist":
		result, err = handleRemoveFromWhitelist(ctx, params.Arguments)
	case "feishu_list_whitelist":
		result, err = handleListWhitelist(ctx, params.Arguments)
	// Keyword management
	case "feishu_add_keyword":
		result, err = handleAddKeyword(ctx, params.Arguments)
	case "feishu_remove_keyword":
		result, err = handleRemoveKeyword(ctx, params.Arguments)
	case "feishu_list_keywords":
		result, err = handleListKeywords(ctx, params.Arguments)
	// Buffer viewing
	case "feishu_get_buffer_summary":
		result, err = handleGetBufferSummary(ctx, params.Arguments)
	case "feishu_get_buffered_messages":
		result, err = handleGetBufferedMessages(ctx, params.Arguments)
	// Interest topic management
	case "feishu_add_interest_topic":
		result, err = handleAddInterestTopic(ctx, params.Arguments)
	case "feishu_remove_interest_topic":
		result, err = handleRemoveInterestTopic(ctx, params.Arguments)
	case "feishu_list_interest_topics":
		result, err = handleListInterestTopics(ctx, params.Arguments)
	default:
		sendError(req.ID, -32601, "Unknown tool", params.Name)
		return
	}

	if err != nil {
		sendToolResult(req.ID, false, err.Error())
		return
	}

	resultJSON, _ := json.Marshal(result)
	sendToolResult(req.ID, true, string(resultJSON))
}

func loadContext() *ChatContext {
	if contextFile == "" {
		return &ChatContext{}
	}

	data, err := os.ReadFile(contextFile)
	if err != nil {
		return &ChatContext{}
	}

	var ctx ChatContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return &ChatContext{}
	}

	return &ctx
}

// IPC request to the parent Bridge process
type IPCRequest struct {
	Action    string                 `json:"action"`
	ChatID    string                 `json:"chat_id"`
	MessageID string                 `json:"message_id"`
	Arguments map[string]interface{} `json:"arguments"`
}

type IPCResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func sendIPCRequest(action string, ctx *ChatContext, args map[string]interface{}) (*IPCResponse, error) {
	if ipcRequestFile == "" || ipcResponseFile == "" {
		return nil, fmt.Errorf("IPC files not configured")
	}

	req := IPCRequest{
		Action:    action,
		ChatID:    ctx.ChatID,
		MessageID: ctx.MessageID,
		Arguments: args,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Clear any stale response file BEFORE writing request (prevents race condition)
	os.Remove(ipcResponseFile)

	// Write request to file
	if err := os.WriteFile(ipcRequestFile, data, 0644); err != nil {
		return nil, err
	}

	// Wait for response (poll with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("IPC timeout")
		case <-ticker.C:
			respData, err := os.ReadFile(ipcResponseFile)
			if err != nil || len(respData) == 0 {
				continue
			}

			// Remove the response file (not just clear it)
			os.Remove(ipcResponseFile)

			var resp IPCResponse
			if err := json.Unmarshal(respData, &resp); err != nil {
				return nil, err
			}

			return &resp, nil
		}
	}
}

func handleGetChatMembers(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	// Read directly from context.json (pre-populated by Bridge)
	// No need for IPC call since Bridge already fetched members when message arrived
	if len(ctx.Members) == 0 {
		return map[string]interface{}{
			"members": []Member{},
			"note":    "No members found in context. This may be a private chat.",
		}, nil
	}
	return map[string]interface{}{
		"members": ctx.Members,
	}, nil
}

func handleGetChatHistory(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Call Bridge to get real chat history from Feishu API
	resp, err := sendIPCRequest("get_chat_history", ctx, map[string]interface{}{
		"limit": limit,
	})
	if err != nil {
		// Fallback to context if IPC fails
		msgs := ctx.RecentMsgs
		if len(msgs) > limit {
			msgs = msgs[len(msgs)-limit:]
		}
		return map[string]interface{}{
			"messages": msgs,
			"source":   "context_fallback",
		}, nil
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

// ========== Whitelist Management ==========

func handleAddToWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		chatID = ctx.ChatID // Default to current chat
	}
	if chatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	reason, _ := args["reason"].(string)
	if reason == "" {
		reason = "Added by Codex"
	}

	resp, err := sendIPCRequest("add_to_whitelist", ctx, map[string]interface{}{
		"chat_id": chatID,
		"reason":  reason,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Chat %s added to whitelist", chatID),
	}, nil
}

func handleRemoveFromWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		chatID = ctx.ChatID
	}
	if chatID == "" {
		return nil, fmt.Errorf("chat_id is required")
	}

	resp, err := sendIPCRequest("remove_from_whitelist", ctx, map[string]interface{}{
		"chat_id": chatID,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Chat %s removed from whitelist", chatID),
	}, nil
}

func handleListWhitelist(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	resp, err := sendIPCRequest("list_whitelist", ctx, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

// ========== Keyword Management ==========

func handleAddKeyword(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	keyword, _ := args["keyword"].(string)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	priority := 1
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}

	resp, err := sendIPCRequest("add_keyword", ctx, map[string]interface{}{
		"keyword":  keyword,
		"priority": priority,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Keyword '%s' added with priority %d", keyword, priority),
	}, nil
}

func handleRemoveKeyword(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	keyword, _ := args["keyword"].(string)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	resp, err := sendIPCRequest("remove_keyword", ctx, map[string]interface{}{
		"keyword": keyword,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Keyword '%s' removed", keyword),
	}, nil
}

func handleListKeywords(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	resp, err := sendIPCRequest("list_keywords", ctx, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

// ========== Buffer Viewing ==========

func handleGetBufferSummary(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	resp, err := sendIPCRequest("get_buffer_summary", ctx, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

func handleGetBufferedMessages(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		chatID = ctx.ChatID
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	resp, err := sendIPCRequest("get_buffered_messages", ctx, map[string]interface{}{
		"chat_id": chatID,
		"limit":   limit,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

func sendResult(id interface{}, result interface{}) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	sendResponse(resp)
}

func sendError(id interface{}, code int, message, data string) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	sendResponse(resp)
}

func sendToolResult(id interface{}, isSuccess bool, content string) {
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
		"isError": !isSuccess,
	}
	sendResult(id, result)
}

func sendResponse(resp MCPResponse) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

// ========== Interest Topic Management ==========

func handleAddInterestTopic(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	resp, err := sendIPCRequest("add_interest_topic", ctx, map[string]interface{}{
		"topic": topic,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Now watching for topic: %s", topic),
	}, nil
}

func handleRemoveInterestTopic(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	resp, err := sendIPCRequest("remove_interest_topic", ctx, map[string]interface{}{
		"topic": topic,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Stopped watching topic: %s", topic),
	}, nil
}

func handleListInterestTopics(ctx *ChatContext, args map[string]interface{}) (interface{}, error) {
	resp, err := sendIPCRequest("list_interest_topics", ctx, nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}
