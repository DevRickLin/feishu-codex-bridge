package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type Client struct {
	appID     string
	appSecret string
	larkCli   *lark.Client
	wsCli     *larkws.Client
	onMessage func(chatID, msgID, content string)
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewClient(appID, appSecret string) *Client {
	return &Client{
		appID:     appID,
		appSecret: appSecret,
	}
}

func (c *Client) OnMessage(handler func(chatID, msgID, content string)) {
	c.onMessage = handler
}

func (c *Client) Start() error {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// 创建 Lark API 客户端
	c.larkCli = lark.NewClient(c.appID, c.appSecret)

	// 注册事件处理器
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			c.handleMessage(event)
			return nil
		})

	// 创建 WebSocket 客户端
	c.wsCli = larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	fmt.Println("[Feishu] Starting WebSocket connection...")

	// 启动 WebSocket（阻塞）
	return c.wsCli.Start(c.ctx)
}

func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Client) handleMessage(event *larkim.P2MessageReceiveV1) {
	msg := event.Event.Message
	if msg == nil {
		return
	}

	// 只处理文本消息
	if *msg.MessageType != "text" {
		return
	}

	// 解析文本内容
	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*msg.Content), &textContent); err != nil {
		fmt.Printf("[Feishu] Failed to parse content: %v\n", err)
		return
	}

	chatID := *msg.ChatId
	msgID := *msg.MessageId
	content := textContent.Text

	fmt.Printf("[Feishu] Received message from chat %s: %s\n", chatID, truncate(content, 50))

	if c.onMessage != nil {
		c.onMessage(chatID, msgID, content)
	}
}

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

func (c *Client) AddReaction(messageID, emoji string) error {
	// 添加表情回应（可选功能）
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
