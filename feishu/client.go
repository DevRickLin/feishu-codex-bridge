package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// Message represents a received Feishu message
type Message struct {
	ChatID      string
	MsgID       string
	MsgType     string            // text, image, post
	ChatType    string            // p2p (private), group
	Content     string            // Text content (extracted from all message types)
	ImageKeys   []string          // Image keys for downloading
	Sender      *Sender           // Message sender info
	Mentions    []string          // Mentioned user IDs (including bot)
	MentionMap  map[string]string // Map from mention key (@_user_1) to real name
	MentionsBot bool              // True if the bot was mentioned
	CreateTime  int64             // Message creation time (milliseconds Unix timestamp from Feishu)
}

// Sender represents the message sender
type Sender struct {
	SenderID   string // User ID or bot ID
	SenderType string // user, bot
	TenantKey  string
}

// ChatMember represents a member in a chat
type ChatMember struct {
	MemberID   string `json:"member_id"`
	MemberType string `json:"member_type"` // user, bot
	Name       string `json:"name"`
}

// ChatInfo represents information about a chat
type ChatInfo struct {
	ChatID      string `json:"chat_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ChatType    string `json:"chat_type"` // p2p, group
	OwnerID     string `json:"owner_id"`
	MemberCount int    `json:"user_count"`
}

// HistoryMessage represents a message from chat history
type HistoryMessage struct {
	MsgID      string `json:"message_id"`
	MsgType    string `json:"msg_type"`
	Content    string `json:"content"`
	CreateTime string `json:"create_time"`
	Sender     *Sender
}

// MessageHandler is the callback for received messages
type MessageHandler func(msg *Message)

// Client is the Feishu API client
type Client struct {
	appID       string
	appSecret   string
	larkCli     *lark.Client
	wsCli       *larkws.Client
	onMessage   MessageHandler
	downloadDir string
	ctx         context.Context
	cancel      context.CancelFunc
	botOpenID   string // Bot's own open_id, learned from first mention
}

// NewClient creates a new Feishu client
func NewClient(appID, appSecret string) *Client {
	return &Client{
		appID:       appID,
		appSecret:   appSecret,
		downloadDir: "/tmp/feishu-images",
	}
}

// SetDownloadDir sets the directory for downloading images
func (c *Client) SetDownloadDir(dir string) {
	c.downloadDir = dir
}

// OnMessage sets the message handler
func (c *Client) OnMessage(handler MessageHandler) {
	c.onMessage = handler
}

// Start connects to Feishu via WebSocket and starts listening for messages
func (c *Client) Start() error {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Create Lark API client
	c.larkCli = lark.NewClient(c.appID, c.appSecret)

	// Fetch bot's own open_id at startup
	if err := c.fetchBotOpenID(); err != nil {
		fmt.Printf("[Feishu] Warning: failed to fetch bot open_id: %v\n", err)
	}

	// Register event handler
	// Note: Must return quickly so SDK can send ACK, otherwise Feishu will retry due to timeout
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			// Process message asynchronously, return immediately to let SDK send ACK
			go c.handleMessage(event)
			return nil
		})

	// Create WebSocket client
	c.wsCli = larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	fmt.Println("[Feishu] Starting WebSocket connection...")

	// Start WebSocket (blocking)
	return c.wsCli.Start(c.ctx)
}

// fetchBotOpenID fetches the bot's own open_id
func (c *Client) fetchBotOpenID() error {
	// 1. First get tenant_access_token
	tokenReq := fmt.Sprintf(`{"app_id":"%s","app_secret":"%s"}`, c.appID, c.appSecret)
	tokenResp, err := http.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json",
		strings.NewReader(tokenReq),
	)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}
	defer tokenResp.Body.Close()

	var tokenResult struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return fmt.Errorf("decode token: %w", err)
	}

	// 2. Get bot info
	req, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/bot/v3/info", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResult.TenantAccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("get bot info: %w", err)
	}
	defer resp.Body.Close()

	var botResult struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&botResult); err != nil {
		return fmt.Errorf("decode bot info: %w", err)
	}

	if botResult.Code != 0 {
		return fmt.Errorf("API error: %s", botResult.Msg)
	}

	c.botOpenID = botResult.Bot.OpenID
	fmt.Printf("[Feishu] Bot open_id: %s (name=%s)\n", c.botOpenID, botResult.Bot.AppName)
	return nil
}

// Stop disconnects from Feishu
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// handleMessage processes incoming Feishu messages
func (c *Client) handleMessage(event *larkim.P2MessageReceiveV1) {
	rawMsg := event.Event.Message
	if rawMsg == nil {
		return
	}

	// Filter out messages sent by the bot itself to prevent infinite loops
	if event.Event.Sender != nil && event.Event.Sender.SenderType != nil {
		if *event.Event.Sender.SenderType == "app" {
			// Message sent by bot, ignore
			return
		}
	}

	msg := &Message{
		ChatID:  *rawMsg.ChatId,
		MsgID:   *rawMsg.MessageId,
		MsgType: *rawMsg.MessageType,
	}

	// Parse create time (milliseconds Unix timestamp)
	if rawMsg.CreateTime != nil {
		if ts, err := strconv.ParseInt(*rawMsg.CreateTime, 10, 64); err == nil {
			msg.CreateTime = ts
		}
	}

	// Parse chat type
	if rawMsg.ChatType != nil {
		msg.ChatType = *rawMsg.ChatType
	}

	// Parse sender info
	if event.Event.Sender != nil {
		msg.Sender = &Sender{}
		if event.Event.Sender.SenderId != nil {
			if event.Event.Sender.SenderId.OpenId != nil {
				msg.Sender.SenderID = *event.Event.Sender.SenderId.OpenId
			}
		}
		if event.Event.Sender.SenderType != nil {
			msg.Sender.SenderType = *event.Event.Sender.SenderType
		}
		if event.Event.Sender.TenantKey != nil {
			msg.Sender.TenantKey = *event.Event.Sender.TenantKey
		}
	}

	// Parse mentions and check if bot was mentioned
	// Also build a map from mention key (@_user_1) to real name
	msg.MentionMap = make(map[string]string)
	if rawMsg.Mentions != nil {
		fmt.Printf("[Feishu] DEBUG: Mentions count=%d\n", len(rawMsg.Mentions))
		for i, mention := range rawMsg.Mentions {
			// Debug: print full mention info
			keyStr := "<nil>"
			nameStr := "<nil>"
			idStr := "<nil>"
			if mention.Key != nil {
				keyStr = *mention.Key
			}
			if mention.Name != nil {
				nameStr = *mention.Name
			}
			if mention.Id != nil && mention.Id.OpenId != nil {
				idStr = *mention.Id.OpenId
			}
			fmt.Printf("[Feishu] DEBUG: Mention[%d] key=%q name=%q id=%q\n", i, keyStr, nameStr, idStr)

			if mention.Id != nil && mention.Id.OpenId != nil {
				openID := *mention.Id.OpenId
				msg.Mentions = append(msg.Mentions, openID)
				// Check if it's the bot
				if openID == c.botOpenID {
					msg.MentionsBot = true
				}
			}
			// Save key -> name mapping for replacing placeholders in messages
			if mention.Key != nil && mention.Name != nil {
				msg.MentionMap[*mention.Key] = *mention.Name
			}
		}
	} else {
		fmt.Printf("[Feishu] DEBUG: Mentions is nil\n")
	}

	switch msg.MsgType {
	case "text":
		msg.Content = c.parseTextContent(*rawMsg.Content, msg.MentionMap)
	case "image":
		msg.ImageKeys = c.parseImageContent(*rawMsg.Content)
		msg.Content = "[Image]"
	case "post":
		content, imageKeys := c.parsePostContent(*rawMsg.Content, msg.MentionMap)
		msg.Content = content
		msg.ImageKeys = imageKeys
	default:
		// Unsupported message type
		fmt.Printf("[Feishu] Unsupported message type: %s\n", msg.MsgType)
		return
	}

	fmt.Printf("[Feishu] Received %s from %s chat %s: %s\n", msg.MsgType, msg.ChatType, msg.ChatID, truncate(msg.Content, 50))

	if c.onMessage != nil {
		c.onMessage(msg)
	}
}

// parseTextContent extracts text from a text message
// It also replaces mention placeholders (@_user_1) with real names
func (c *Client) parseTextContent(content string, mentionMap map[string]string) string {
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return replaceMentions(parsed.Text, mentionMap)
}

// parseImageContent extracts image key from an image message
func (c *Client) parseImageContent(content string) []string {
	var parsed struct {
		ImageKey string `json:"image_key"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil
	}
	if parsed.ImageKey == "" {
		return nil
	}
	return []string{parsed.ImageKey}
}

// parsePostContent extracts text and images from a rich text message
// It also replaces mention placeholders (@_user_1) with real names
func (c *Client) parsePostContent(content string, mentionMap map[string]string) (string, []string) {
	var parsed struct {
		Title   string `json:"title"`
		Content [][]struct {
			Tag      string `json:"tag"`
			Text     string `json:"text,omitempty"`
			ImageKey string `json:"image_key,omitempty"`
			UserID   string `json:"user_id,omitempty"` // for "at" tags
		} `json:"content"`
	}

	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", nil
	}

	var textParts []string
	var imageKeys []string

	if parsed.Title != "" {
		textParts = append(textParts, parsed.Title)
	}

	for _, line := range parsed.Content {
		var lineParts []string
		for _, elem := range line {
			switch elem.Tag {
			case "text":
				if elem.Text != "" {
					lineParts = append(lineParts, elem.Text)
				}
			case "at":
				// For "at" tags in post content, try to find the name from mentionMap
				// The user_id might be in the form "@_user_1" or just the open_id
				if elem.UserID != "" {
					if name, ok := mentionMap[elem.UserID]; ok {
						lineParts = append(lineParts, "@"+name)
					} else {
						// If not found in map, just use @user_id as fallback
						lineParts = append(lineParts, "@"+elem.UserID)
					}
				}
			case "img":
				if elem.ImageKey != "" {
					imageKeys = append(imageKeys, elem.ImageKey)
				}
			}
		}
		if len(lineParts) > 0 {
			textParts = append(textParts, joinStrings(lineParts, ""))
		}
	}

	result := joinStrings(textParts, "\n")
	// Also replace any remaining mention placeholders in the text
	result = replaceMentions(result, mentionMap)
	return result, imageKeys
}

// replaceMentions replaces mention placeholders (@_user_1, @_user_2, etc.) with real names
func replaceMentions(text string, mentionMap map[string]string) string {
	if len(mentionMap) == 0 {
		return text
	}
	result := text
	for key, name := range mentionMap {
		// Replace @_user_1 with @RealName
		result = strings.ReplaceAll(result, key, "@"+name)
	}
	return result
}

// DownloadImage downloads an image from Feishu and saves it locally
func (c *Client) DownloadImage(messageID, imageKey string) (string, error) {
	// Ensure download directory exists
	if err := os.MkdirAll(c.downloadDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create download dir: %w", err)
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(imageKey).
		Type("image").
		Build()

	resp, err := c.larkCli.Im.MessageResource.Get(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to get image: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("get image error: %s", resp.Msg)
	}

	// Save to file
	filePath := filepath.Join(c.downloadDir, imageKey+".png")
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.File)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("[Feishu] Downloaded image to %s\n", filePath)
	return filePath, nil
}

// Mention represents a user to be mentioned in a message
type Mention struct {
	UserID   string // open_id (ou_xxx) or user_id
	UserName string // Display name for the mention
}

// SendText sends a text message to a chat
func (c *Client) SendText(chatID, text string) error {
	content := map[string]string{"text": text}
	contentJSON, _ := json.Marshal(content)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(context.Background(), req)
	if err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Message sent to %s\n", chatID)
	return nil
}

// SendTextWithMentions sends a text message with @ mentions
// Format: text with <at user_id="ou_xxx">@name</at> tags
func (c *Client) SendTextWithMentions(chatID, text string, mentions []Mention) error {
	// Build text with mention tags
	mentionText := text
	for _, m := range mentions {
		// Append mention tag at the beginning or where specified
		tag := fmt.Sprintf("<at user_id=\"%s\">@%s</at>", m.UserID, m.UserName)
		mentionText = tag + " " + mentionText
	}

	content := map[string]string{"text": mentionText}
	contentJSON, _ := json.Marshal(content)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(context.Background(), req)
	if err != nil {
		return fmt.Errorf("send message with mentions failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message with mentions error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Message with %d mentions sent to %s\n", len(mentions), chatID)
	return nil
}

// SendTextMentionAll sends a text message that mentions all members (@all)
func (c *Client) SendTextMentionAll(chatID, text string) error {
	// Use <at user_id="all">@all</at> to mention everyone
	mentionText := "<at user_id=\"all\">@all</at> " + text

	content := map[string]string{"text": mentionText}
	contentJSON, _ := json.Marshal(content)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(context.Background(), req)
	if err != nil {
		return fmt.Errorf("send message mention all failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message mention all error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Message with @all sent to %s\n", chatID)
	return nil
}

// SendRichText sends a rich text (post) message to a chat
func (c *Client) SendRichText(chatID, title string, content [][]map[string]interface{}) error {
	post := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"title":   title,
			"content": content,
		},
	}
	contentJSON, _ := json.Marshal(post)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypePost).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.Message.Create(context.Background(), req)
	if err != nil {
		return fmt.Errorf("send rich text failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send rich text error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Rich text sent to %s\n", chatID)
	return nil
}

// AddReaction adds an emoji reaction to a message
func (c *Client) AddReaction(messageID, emojiType string) error {
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emojiType).Build()).
			Build()).
		Build()

	resp, err := c.larkCli.Im.MessageReaction.Create(context.Background(), req)
	if err != nil {
		return fmt.Errorf("add reaction failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("add reaction error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Reaction %s added to message %s\n", emojiType, messageID)
	return nil
}

// RemoveReaction removes an emoji reaction from a message
func (c *Client) RemoveReaction(messageID, reactionID string) error {
	req := larkim.NewDeleteMessageReactionReqBuilder().
		MessageId(messageID).
		ReactionId(reactionID).
		Build()

	resp, err := c.larkCli.Im.MessageReaction.Delete(context.Background(), req)
	if err != nil {
		return fmt.Errorf("remove reaction failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("remove reaction error: %s", resp.Msg)
	}

	fmt.Printf("[Feishu] Reaction removed from message %s\n", messageID)
	return nil
}

// GetChatHistory retrieves recent messages from a chat
// pageSize: number of messages to retrieve (max 50)
// Returns messages in chronological order (oldest first, newest last)
func (c *Client) GetChatHistory(chatID string, pageSize int) ([]*HistoryMessage, error) {
	if pageSize > 50 {
		pageSize = 50
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// Use ByCreateTimeDesc to get latest messages (descending: newest first)
	// Feishu API defaults to ascending (oldest first), which would return old messages from when group was created
	req := larkim.NewListMessageReqBuilder().
		ContainerIdType("chat").
		ContainerId(chatID).
		SortType("ByCreateTimeDesc"). // Key: descending order for latest messages
		PageSize(pageSize).
		Build()

	resp, err := c.larkCli.Im.Message.List(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("get chat history failed: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("get chat history error: %s", resp.Msg)
	}

	var messages []*HistoryMessage
	for _, item := range resp.Data.Items {
		msg := &HistoryMessage{
			MsgID:      *item.MessageId,
			MsgType:    *item.MsgType,
			CreateTime: *item.CreateTime,
		}

		// Build mention map from the message's mentions (if any)
		// This allows us to replace @_user_N placeholders with real names
		mentionMap := make(map[string]string)
		if item.Mentions != nil {
			for _, mention := range item.Mentions {
				if mention.Key != nil && mention.Name != nil {
					mentionMap[*mention.Key] = *mention.Name
				}
			}
		}

		// Parse content based on message type, resolving mention placeholders
		if item.Body != nil && item.Body.Content != nil {
			rawContent := *item.Body.Content
			switch *item.MsgType {
			case "text":
				msg.Content = c.parseTextContent(rawContent, mentionMap)
			case "post":
				content, _ := c.parsePostContent(rawContent, mentionMap)
				msg.Content = content
			default:
				msg.Content = rawContent
			}
		}

		// Parse sender
		if item.Sender != nil {
			msg.Sender = &Sender{}
			if item.Sender.Id != nil {
				msg.Sender.SenderID = *item.Sender.Id
			}
			if item.Sender.SenderType != nil {
				msg.Sender.SenderType = *item.Sender.SenderType
			}
			if item.Sender.TenantKey != nil {
				msg.Sender.TenantKey = *item.Sender.TenantKey
			}
		}

		messages = append(messages, msg)
	}

	// Reverse to chronological order (oldest first, newest last) for easier reading
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	fmt.Printf("[Feishu] Retrieved %d messages from chat %s\n", len(messages), chatID)
	return messages, nil
}

// GetChatMembers retrieves members of a chat (group)
// Uses pagination to get all members
func (c *Client) GetChatMembers(chatID string) ([]*ChatMember, error) {
	var members []*ChatMember
	var pageToken string

	for {
		reqBuilder := larkim.NewGetChatMembersReqBuilder().
			MemberIdType("open_id"). // Request open_id format for user IDs
			ChatId(chatID).
			PageSize(100) // Max page size

		if pageToken != "" {
			reqBuilder = reqBuilder.PageToken(pageToken)
		}

		req := reqBuilder.Build()
		resp, err := c.larkCli.Im.ChatMembers.Get(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("get chat members failed: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("get chat members error: %s", resp.Msg)
		}

		for _, item := range resp.Data.Items {
			member := &ChatMember{}
			if item.MemberId != nil {
				member.MemberID = *item.MemberId
			}
			if item.MemberIdType != nil {
				member.MemberType = *item.MemberIdType
			}
			if item.Name != nil {
				member.Name = *item.Name
			}
			members = append(members, member)
		}

		// Check if there are more pages
		if resp.Data.PageToken == nil || *resp.Data.PageToken == "" {
			break
		}
		pageToken = *resp.Data.PageToken
	}

	fmt.Printf("[Feishu] Retrieved %d members from chat %s\n", len(members), chatID)
	return members, nil
}

// GetChatInfo retrieves information about a chat
func (c *Client) GetChatInfo(chatID string) (*ChatInfo, error) {
	req := larkim.NewGetChatReqBuilder().
		ChatId(chatID).
		Build()

	resp, err := c.larkCli.Im.Chat.Get(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("get chat info failed: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("get chat info error: %s", resp.Msg)
	}

	info := &ChatInfo{
		ChatID: chatID,
	}
	if resp.Data.Name != nil {
		info.Name = *resp.Data.Name
	}
	if resp.Data.Description != nil {
		info.Description = *resp.Data.Description
	}
	if resp.Data.ChatMode != nil {
		info.ChatType = *resp.Data.ChatMode
	}
	if resp.Data.OwnerId != nil {
		info.OwnerID = *resp.Data.OwnerId
	}
	if resp.Data.UserCount != nil {
		var count int
		fmt.Sscanf(*resp.Data.UserCount, "%d", &count)
		info.MemberCount = count
	}

	fmt.Printf("[Feishu] Got chat info for %s: %s (%d members)\n", chatID, info.Name, info.MemberCount)
	return info, nil
}

// FormatHistoryAsContext formats chat history as context string for AI
func FormatHistoryAsContext(messages []*HistoryMessage, maxMessages int) string {
	if len(messages) == 0 {
		return ""
	}

	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[:maxMessages]
	}

	var parts []string
	// Messages are usually newest first, so reverse for chronological order
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		senderType := "User"
		if msg.Sender != nil && msg.Sender.SenderType == "bot" {
			senderType = "Bot"
		}

		// Extract text from content JSON
		content := msg.Content
		if msg.MsgType == "text" {
			var parsed struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(content), &parsed); err == nil {
				content = parsed.Text
			}
		}

		parts = append(parts, fmt.Sprintf("[%s]: %s", senderType, content))
	}

	return joinStrings(parts, "\n")
}

// Helper functions

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
