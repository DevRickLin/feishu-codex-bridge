package data

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/anthropics/feishu-codex-bridge/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
)

// feishuRepo implements the Feishu message repository
type feishuRepo struct {
	client *feishu.Client
}

// NewFeishuRepo creates a new Feishu repository
func NewFeishuRepo(client *feishu.Client) repo.MessageRepo {
	return &feishuRepo{client: client}
}

// GetChatHistory gets chat history
func (r *feishuRepo) GetChatHistory(ctx context.Context, chatID string, limit int) ([]domain.Message, error) {
	msgs, err := r.client.GetChatHistory(chatID, limit)
	if err != nil {
		return nil, err
	}

	// Get member list for resolving sender names
	members, _ := r.client.GetChatMembers(chatID)
	memberMap := make(map[string]string)
	for _, m := range members {
		memberMap[m.MemberID] = m.Name
	}

	var result []domain.Message
	for _, m := range msgs {
		createTime := time.Now()
		if m.CreateTime != "" {
			// Feishu timestamp is millisecond string
			if ms, err := strconv.ParseInt(m.CreateTime, 10, 64); err == nil {
				createTime = time.UnixMilli(ms)
			}
		}

		// Extract sender info
		senderID := ""
		senderName := ""
		isBot := false
		if m.Sender != nil {
			senderID = m.Sender.SenderID
			senderName = memberMap[senderID]
			isBot = m.Sender.SenderType == "bot"
		}

		// Message content already parsed in feishu.Client.GetChatHistory (including @ mention replacement)
		// Use m.Content directly

		result = append(result, domain.Message{
			ID:         m.MsgID,
			ChatID:     chatID,
			Content:    m.Content,
			SenderID:   senderID,
			SenderName: senderName,
			MsgType:    m.MsgType,
			CreateTime: createTime,
			IsBot:      isBot,
		})
	}
	return result, nil
}

// GetChatMembers gets chat member list
func (r *feishuRepo) GetChatMembers(ctx context.Context, chatID string) ([]domain.Member, error) {
	members, err := r.client.GetChatMembers(chatID)
	if err != nil {
		return nil, err
	}

	var result []domain.Member
	for _, m := range members {
		result = append(result, domain.Member{
			UserID: m.MemberID,
			Name:   m.Name,
		})
	}
	return result, nil
}

// GetChatInfo gets chat info
func (r *feishuRepo) GetChatInfo(ctx context.Context, chatID string) (*repo.ChatInfo, error) {
	// Simple implementation: determine type by chatID prefix
	chatType := domain.ChatTypeP2P
	if len(chatID) > 3 && chatID[:3] == "oc_" {
		chatType = domain.ChatTypeGroup
	}

	return &repo.ChatInfo{
		ChatID:   chatID,
		ChatType: chatType,
	}, nil
}

// SendText sends a text message
func (r *feishuRepo) SendText(ctx context.Context, chatID, text string) error {
	return r.client.SendText(chatID, text)
}

// SendTextWithMentions sends a text message with @ mentions
func (r *feishuRepo) SendTextWithMentions(ctx context.Context, chatID, text string, mentions []domain.Member) error {
	var feishuMentions []feishu.Mention
	for _, m := range mentions {
		feishuMentions = append(feishuMentions, feishu.Mention{
			UserID:   m.UserID,
			UserName: m.Name,
		})
	}
	return r.client.SendTextWithMentions(chatID, text, feishuMentions)
}

// AddReaction adds an emoji reaction
func (r *feishuRepo) AddReaction(ctx context.Context, msgID, reactionType string) error {
	return r.client.AddReaction(msgID, reactionType)
}

// parseMessageContent parses message content, extracting text from JSON
func parseMessageContent(msgType, rawContent string) string {
	switch msgType {
	case "text":
		var parsed struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(rawContent), &parsed); err == nil {
			return parsed.Text
		}
	case "post":
		// Rich text message
		var parsed struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text,omitempty"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(rawContent), &parsed); err == nil {
			var parts []string
			if parsed.Title != "" {
				parts = append(parts, parsed.Title)
			}
			for _, line := range parsed.Content {
				var lineParts []string
				for _, elem := range line {
					if elem.Tag == "text" && elem.Text != "" {
						lineParts = append(lineParts, elem.Text)
					}
				}
				if len(lineParts) > 0 {
					parts = append(parts, joinStrings(lineParts, ""))
				}
			}
			return joinStrings(parts, "\n")
		}
	case "image":
		return "[Image]"
	case "file":
		return "[File]"
	case "audio":
		return "[Audio]"
	case "video":
		return "[Video]"
	case "sticker":
		return "[Sticker]"
	case "interactive":
		return "[Card Message]"
	}
	// Default: return raw content
	return rawContent
}

// joinStrings joins strings
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
