package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FeishuMCPServer provides MCP tools for interacting with Feishu
type FeishuMCPServer struct {
	server      *mcp.Server
	chatContext *ChatContext
	mu          sync.RWMutex
}

// ChatContext holds the current chat context for the MCP server
type ChatContext struct {
	ChatID     string
	ChatType   string   // "group" or "p2p"
	Members    []Member // Group members
	RecentMsgs []RecentMessage
}

// Member represents a chat member
type Member struct {
	ID   string
	Name string
}

// RecentMessage represents a recent message in the chat
type RecentMessage struct {
	Sender    string
	Content   string
	Timestamp int64
}

// MessageCallback is called when the agent wants to send a message
type MessageCallback func(chatID, content string, mentions []string, mentionAll bool) error

// ReactionCallback is called when the agent wants to add a reaction
type ReactionCallback func(msgID, emojiType string) error

// Callbacks holds the callback functions for MCP tools
type Callbacks struct {
	SendMessage MessageCallback
	AddReaction ReactionCallback
}

var (
	globalServer    *FeishuMCPServer
	globalCallbacks *Callbacks
	serverMu        sync.Mutex
)

// NewServer creates a new Feishu MCP server
func NewServer(callbacks *Callbacks) *FeishuMCPServer {
	serverMu.Lock()
	defer serverMu.Unlock()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "feishu-tools",
		Version: "v1.0.0",
	}, nil)

	fs := &FeishuMCPServer{
		server:      server,
		chatContext: &ChatContext{},
	}

	globalServer = fs
	globalCallbacks = callbacks

	// Register tools
	fs.registerTools()

	return fs
}

// SetChatContext updates the current chat context
func (s *FeishuMCPServer) SetChatContext(ctx *ChatContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatContext = ctx
}

// GetChatContext returns the current chat context
func (s *FeishuMCPServer) GetChatContext() *ChatContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatContext
}

// registerTools registers all Feishu-related MCP tools
func (s *FeishuMCPServer) registerTools() {
	// Tool: send_message - Send a message to the current chat
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_send_message",
		Description: "Send a message to the current Feishu chat. Use this to reply to users or send notifications.",
	}, handleSendMessage)

	// Tool: add_reaction - Add emoji reaction to a message
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_add_reaction",
		Description: "Add an emoji reaction to a message. Available emojis: THUMBSUP, DONE, HEART, APPRECIATE, LAUGH, JIAYI, FINGERHEART, SURPRISED, CRY, PARTY, EMBARRASSED",
	}, handleAddReaction)

	// Tool: get_chat_members - Get members of the current group chat
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_get_chat_members",
		Description: "Get the list of members in the current group chat. Returns member IDs and names for @mentioning.",
	}, handleGetChatMembers)

	// Tool: get_chat_history - Get recent messages in the chat
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_get_chat_history",
		Description: "Get recent messages from the current chat for context. Use this to understand the conversation history.",
	}, handleGetChatHistory)

	// Tool: mention_user - Mention a specific user in a message
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_mention_user",
		Description: "Send a message that @mentions a specific user. Provide the user_id and message content.",
	}, handleMentionUser)

	// Tool: mention_all - Mention all members in a group chat
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "feishu_mention_all",
		Description: "Send a message that @mentions all members in the group chat. Use sparingly for important announcements.",
	}, handleMentionAll)
}

// SendMessageInput is the input for send_message tool
type SendMessageInput struct {
	Content string `json:"content" jsonschema:"description=The message content to send"`
}

// SendMessageOutput is the output for send_message tool
type SendMessageOutput struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func handleSendMessage(ctx context.Context, req *mcp.CallToolRequest, input SendMessageInput) (*mcp.CallToolResult, SendMessageOutput, error) {
	if globalCallbacks == nil || globalCallbacks.SendMessage == nil {
		return nil, SendMessageOutput{Success: false, Error: "callback not configured"}, nil
	}

	chatCtx := globalServer.GetChatContext()
	if chatCtx.ChatID == "" {
		return nil, SendMessageOutput{Success: false, Error: "no active chat"}, nil
	}

	err := globalCallbacks.SendMessage(chatCtx.ChatID, input.Content, nil, false)
	if err != nil {
		return nil, SendMessageOutput{Success: false, Error: err.Error()}, nil
	}

	return nil, SendMessageOutput{Success: true}, nil
}

// AddReactionInput is the input for add_reaction tool
type AddReactionInput struct {
	MessageID string `json:"message_id" jsonschema:"description=The ID of the message to react to. Use 'current' for the current message."`
	EmojiType string `json:"emoji_type" jsonschema:"description=The emoji type: THUMBSUP, DONE, HEART, APPRECIATE, LAUGH, JIAYI, FINGERHEART, SURPRISED, CRY, PARTY, EMBARRASSED"`
}

// AddReactionOutput is the output for add_reaction tool
type AddReactionOutput struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func handleAddReaction(ctx context.Context, req *mcp.CallToolRequest, input AddReactionInput) (*mcp.CallToolResult, AddReactionOutput, error) {
	if globalCallbacks == nil || globalCallbacks.AddReaction == nil {
		return nil, AddReactionOutput{Success: false, Error: "callback not configured"}, nil
	}

	err := globalCallbacks.AddReaction(input.MessageID, input.EmojiType)
	if err != nil {
		return nil, AddReactionOutput{Success: false, Error: err.Error()}, nil
	}

	return nil, AddReactionOutput{Success: true}, nil
}

// GetChatMembersInput is empty - no input needed
type GetChatMembersInput struct{}

// GetChatMembersOutput contains the list of members
type GetChatMembersOutput struct {
	Members []Member `json:"members"`
	Error   string   `json:"error,omitempty"`
}

func handleGetChatMembers(ctx context.Context, req *mcp.CallToolRequest, input GetChatMembersInput) (*mcp.CallToolResult, GetChatMembersOutput, error) {
	chatCtx := globalServer.GetChatContext()
	if chatCtx.ChatID == "" {
		return nil, GetChatMembersOutput{Error: "no active chat"}, nil
	}

	return nil, GetChatMembersOutput{Members: chatCtx.Members}, nil
}

// GetChatHistoryInput specifies how many messages to retrieve
type GetChatHistoryInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=Maximum number of messages to retrieve (default 20)"`
}

// GetChatHistoryOutput contains recent messages
type GetChatHistoryOutput struct {
	Messages []RecentMessage `json:"messages"`
	Error    string          `json:"error,omitempty"`
}

func handleGetChatHistory(ctx context.Context, req *mcp.CallToolRequest, input GetChatHistoryInput) (*mcp.CallToolResult, GetChatHistoryOutput, error) {
	chatCtx := globalServer.GetChatContext()
	if chatCtx.ChatID == "" {
		return nil, GetChatHistoryOutput{Error: "no active chat"}, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	messages := chatCtx.RecentMsgs
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return nil, GetChatHistoryOutput{Messages: messages}, nil
}

// MentionUserInput is the input for mentioning a specific user
type MentionUserInput struct {
	UserID  string `json:"user_id" jsonschema:"description=The user_id of the member to mention"`
	Content string `json:"content" jsonschema:"description=The message content"`
}

// MentionUserOutput is the output
type MentionUserOutput struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func handleMentionUser(ctx context.Context, req *mcp.CallToolRequest, input MentionUserInput) (*mcp.CallToolResult, MentionUserOutput, error) {
	if globalCallbacks == nil || globalCallbacks.SendMessage == nil {
		return nil, MentionUserOutput{Success: false, Error: "callback not configured"}, nil
	}

	chatCtx := globalServer.GetChatContext()
	if chatCtx.ChatID == "" {
		return nil, MentionUserOutput{Success: false, Error: "no active chat"}, nil
	}

	err := globalCallbacks.SendMessage(chatCtx.ChatID, input.Content, []string{input.UserID}, false)
	if err != nil {
		return nil, MentionUserOutput{Success: false, Error: err.Error()}, nil
	}

	return nil, MentionUserOutput{Success: true}, nil
}

// MentionAllInput is the input for mentioning all members
type MentionAllInput struct {
	Content string `json:"content" jsonschema:"description=The message content"`
}

// MentionAllOutput is the output
type MentionAllOutput struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func handleMentionAll(ctx context.Context, req *mcp.CallToolRequest, input MentionAllInput) (*mcp.CallToolResult, MentionAllOutput, error) {
	if globalCallbacks == nil || globalCallbacks.SendMessage == nil {
		return nil, MentionAllOutput{Success: false, Error: "callback not configured"}, nil
	}

	chatCtx := globalServer.GetChatContext()
	if chatCtx.ChatID == "" {
		return nil, MentionAllOutput{Success: false, Error: "no active chat"}, nil
	}

	err := globalCallbacks.SendMessage(chatCtx.ChatID, input.Content, nil, true)
	if err != nil {
		return nil, MentionAllOutput{Success: false, Error: err.Error()}, nil
	}

	return nil, MentionAllOutput{Success: true}, nil
}

// GetToolsJSON returns the tools as JSON for system prompt injection
func (s *FeishuMCPServer) GetToolsJSON() string {
	tools := []map[string]interface{}{
		{
			"name":        "feishu_send_message",
			"description": "Send a message to the current Feishu chat",
			"parameters": map[string]interface{}{
				"content": "string - The message content to send",
			},
		},
		{
			"name":        "feishu_add_reaction",
			"description": "Add an emoji reaction to a message",
			"parameters": map[string]interface{}{
				"message_id": "string - The message ID (use 'current' for current message)",
				"emoji_type": "string - THUMBSUP, DONE, HEART, APPRECIATE, LAUGH, JIAYI, FINGERHEART, SURPRISED, CRY, PARTY, EMBARRASSED",
			},
		},
		{
			"name":        "feishu_get_chat_members",
			"description": "Get the list of members in the current group chat",
			"parameters":  map[string]interface{}{},
		},
		{
			"name":        "feishu_get_chat_history",
			"description": "Get recent messages from the current chat",
			"parameters": map[string]interface{}{
				"limit": "int (optional) - Maximum number of messages (default 20)",
			},
		},
		{
			"name":        "feishu_mention_user",
			"description": "Send a message that @mentions a specific user",
			"parameters": map[string]interface{}{
				"user_id": "string - The user_id of the member to mention",
				"content": "string - The message content",
			},
		},
		{
			"name":        "feishu_mention_all",
			"description": "Send a message that @mentions all members",
			"parameters": map[string]interface{}{
				"content": "string - The message content",
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(tools, "", "  ")
	return string(jsonBytes)
}

// Run starts the MCP server with stdio transport
func (s *FeishuMCPServer) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// GetServer returns the underlying MCP server
func (s *FeishuMCPServer) GetServer() *mcp.Server {
	return s.server
}

// FormatHistoryForPrompt formats the chat history for injection into the system prompt
func FormatHistoryForPrompt(messages []RecentMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var result string
	result = "[Chat messages for context]\n"
	for _, msg := range messages {
		result += fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content)
	}
	result += "[/Chat messages]\n"

	return result
}
