package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is the HTTP client for communicating with Bridge API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new MCP client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ChatContext holds the current chat context
type ChatContext struct {
	ChatID    string   `json:"chat_id"`
	ChatType  string   `json:"chat_type"`
	MessageID string   `json:"message_id"`
	Members   []Member `json:"members"`
}

// Member represents a chat member
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Message represents a chat message
type Message struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	CreateTime string `json:"create_time"`
}

// ============ Context ============

// GetContext gets the current chat context
func (c *Client) GetContext() (*ChatContext, error) {
	var ctx ChatContext
	if err := c.get("/api/context", &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// ============ Chat Operations ============

// GetChatMembers gets members of a chat
func (c *Client) GetChatMembers(chatID string) ([]Member, error) {
	var result struct {
		Members []Member `json:"members"`
	}
	if err := c.get(fmt.Sprintf("/api/chat/%s/members", chatID), &result); err != nil {
		return nil, err
	}
	return result.Members, nil
}

// GetChatHistory gets chat history
func (c *Client) GetChatHistory(chatID string, limit int) ([]Message, error) {
	var result struct {
		Messages []Message `json:"messages"`
	}
	path := fmt.Sprintf("/api/chat/%s/history?limit=%d", chatID, limit)
	if err := c.get(path, &result); err != nil {
		return nil, err
	}
	return result.Messages, nil
}

// ============ Whitelist Operations ============

// WhitelistEntry represents a whitelist entry
type WhitelistEntry struct {
	ChatID    string `json:"chat_id"`
	Reason    string `json:"reason"`
	AddedBy   string `json:"added_by"`
	CreatedAt string `json:"created_at"`
}

// GetWhitelist gets all whitelisted chats
func (c *Client) GetWhitelist() ([]WhitelistEntry, error) {
	var result struct {
		Entries []WhitelistEntry `json:"entries"`
	}
	if err := c.get("/api/whitelist", &result); err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// AddToWhitelist adds a chat to whitelist
func (c *Client) AddToWhitelist(chatID, reason string) error {
	body := map[string]string{"chat_id": chatID, "reason": reason}
	return c.post("/api/whitelist", body, nil)
}

// RemoveFromWhitelist removes a chat from whitelist
func (c *Client) RemoveFromWhitelist(chatID string) error {
	return c.delete(fmt.Sprintf("/api/whitelist/%s", url.PathEscape(chatID)))
}

// ============ Keyword Operations ============

// Keyword represents a trigger keyword
type Keyword struct {
	Keyword   string `json:"keyword"`
	Priority  int    `json:"priority"`
	CreatedAt string `json:"created_at"`
}

// GetKeywords gets all keywords
func (c *Client) GetKeywords() ([]Keyword, error) {
	var result struct {
		Keywords []Keyword `json:"keywords"`
	}
	if err := c.get("/api/keywords", &result); err != nil {
		return nil, err
	}
	return result.Keywords, nil
}

// AddKeyword adds a keyword
func (c *Client) AddKeyword(keyword string, priority int) error {
	body := map[string]interface{}{"keyword": keyword, "priority": priority}
	return c.post("/api/keywords", body, nil)
}

// RemoveKeyword removes a keyword
func (c *Client) RemoveKeyword(keyword string) error {
	return c.delete(fmt.Sprintf("/api/keywords/%s", url.PathEscape(keyword)))
}

// ============ Buffer Operations ============

// BufferSummary represents buffer summary for a chat
type BufferSummary struct {
	ChatID       string `json:"chat_id"`
	MessageCount int    `json:"message_count"`
}

// GetBufferSummary gets buffer summary
func (c *Client) GetBufferSummary() ([]BufferSummary, error) {
	var result struct {
		Summaries []BufferSummary `json:"summaries"`
	}
	if err := c.get("/api/buffer/summary", &result); err != nil {
		return nil, err
	}
	return result.Summaries, nil
}

// BufferedMessage represents a buffered message
type BufferedMessage struct {
	ID         int64  `json:"id"`
	ChatID     string `json:"chat_id"`
	Content    string `json:"content"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	CreatedAt  string `json:"created_at"`
}

// GetBufferedMessages gets buffered messages for a chat
func (c *Client) GetBufferedMessages(chatID string, limit int) ([]BufferedMessage, error) {
	var result struct {
		Messages []BufferedMessage `json:"messages"`
	}
	path := fmt.Sprintf("/api/buffer/%s/messages?limit=%d", chatID, limit)
	if err := c.get(path, &result); err != nil {
		return nil, err
	}
	return result.Messages, nil
}

// ============ Topic Operations ============

// Topic represents an interest topic
type Topic struct {
	Topic       string `json:"topic"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

// GetTopics gets all interest topics
func (c *Client) GetTopics() ([]Topic, error) {
	var result struct {
		Topics []Topic `json:"topics"`
	}
	if err := c.get("/api/topics", &result); err != nil {
		return nil, err
	}
	return result.Topics, nil
}

// AddTopic adds an interest topic
func (c *Client) AddTopic(topic string) error {
	body := map[string]string{"topic": topic}
	return c.post("/api/topics", body, nil)
}

// RemoveTopic removes an interest topic
func (c *Client) RemoveTopic(topic string) error {
	return c.delete(fmt.Sprintf("/api/topics/%s", url.PathEscape(topic)))
}

// ============ HTTP Helpers ============

func (c *Client) get(path string, result interface{}) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) post(path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("HTTP POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP DELETE failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
