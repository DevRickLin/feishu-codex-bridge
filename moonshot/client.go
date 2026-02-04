package moonshot

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	moonshotBaseURL = "https://api.moonshot.cn/v1"
)

// Client is the Moonshot API client using OpenAI-compatible interface
type Client struct {
	client *openai.Client
	model  string
}

// NewClient creates a new Moonshot client
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "moonshot-v1-8k"
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = moonshotBaseURL

	return &Client{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// Chat sends a message and returns the response
func (c *Client) Chat(systemPrompt, userMessage string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
		Temperature: 0.1, // Low temperature for deterministic responses
		MaxTokens:   50,  // Short response needed for YES/NO
	})
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	return resp.Choices[0].Message.Content, nil
}

// GetListenStrategyWithBotName returns the listen strategy prompt with the bot name included
func GetListenStrategyWithBotName(botName string) string {
	return GetListenStrategyWithTopics(botName, nil)
}

// GetListenStrategyWithTopics returns the listen strategy prompt with bot name and interest topics
func GetListenStrategyWithTopics(botName string, topics []string) string {
	topicsSection := ""
	if len(topics) > 0 {
		topicsSection = fmt.Sprintf(`
- Topics of interest: %s (if related to these topics -> YES)`, strings.Join(topics, ", "))
	}

	return fmt.Sprintf(`You are a message filter that determines whether group chat messages need a response from the bot "%s".

## Bot Information
- Name: %s
- Role: Programming assistant, skilled at code, technical questions, and file operations%s

## Recognizing @ Mentions
Messages can contain @ mentions in two formats:
1. @%s (using the bot name directly) -> Clearly calling the bot
2. @_user_1, @_user_2 placeholders -> System format, usually **NOT** the bot

**Key**: @_user_N format mentions are usually @ other users in the group, not the bot.
Only explicit @%s means they are calling the bot.

## Decision Rules
1. Message explicitly contains @%s -> YES
2. Message contains @_user_N (placeholder format) -> This is @ other users -> NO
3. Message has no @, but is asking technical/programming questions -> YES
4. Message is casual chat, unrelated to tech -> NO
5. Uncertain -> NO

Reply only YES or NO.`, botName, botName, topicsSection, botName, botName, botName)
}

// DefaultListenStrategy is the default prompt for determining if bot should respond (without bot name)
const DefaultListenStrategy = `You are a message filter that determines whether group chat messages need a response from the bot.

The bot is a programming assistant, skilled at code, technical questions, file operations, etc.

Decision rules:
1. If the message is asking technical/programming questions -> YES
2. If the message is casual chat, unrelated to the bot -> NO
3. If the message is explicitly calling/asking the bot -> YES
4. If uncertain -> NO

Reply only "YES" or "NO", no explanations.`

// ShouldRespond determines if the bot should respond to a message
func (c *Client) ShouldRespond(message, botName string, recentContext string) (bool, string) {
	// Use bot name in the strategy if provided
	strategy := ""
	if botName != "" {
		strategy = GetListenStrategyWithBotName(botName)
	}
	return c.ShouldRespondWithStrategy(message, recentContext, strategy)
}

// ShouldRespondWithStrategy determines if the bot should respond using a custom strategy
// If strategy is empty, uses DefaultListenStrategy
func (c *Client) ShouldRespondWithStrategy(message, recentContext, strategy string) (bool, string) {
	systemPrompt := strategy
	if systemPrompt == "" {
		systemPrompt = DefaultListenStrategy
	}

	var userMsg string
	if recentContext != "" {
		userMsg = fmt.Sprintf("## Recent chat history\n%s\n\n## Message to evaluate\n%s", recentContext, message)
	} else {
		userMsg = message
	}

	resp, err := c.Chat(systemPrompt, userMsg)
	if err != nil {
		// On error, default to not responding (conservative)
		fmt.Printf("[Moonshot] Error checking relevance: %v\n", err)
		return false, ""
	}

	resp = strings.TrimSpace(resp)
	shouldRespond := strings.HasPrefix(strings.ToUpper(resp), "YES")
	fmt.Printf("[Moonshot] Response: %q -> shouldRespond=%v\n", resp, shouldRespond)
	return shouldRespond, resp
}

// SummarizeChatHistory summarizes recent chat history for context injection
// Returns a concise summary that can be injected into the prompt without using too many tokens
func (c *Client) SummarizeChatHistory(history string) (string, error) {
	if history == "" {
		return "", nil
	}

	systemPrompt := `You are a conversation summarizer. Please summarize the chat history into a brief context description.

Requirements:
1. Keep key information: who said what important things, topics discussed, unresolved questions
2. Use third-person description
3. Keep it under 200 words
4. If the conversation is simple or has no important content, make it shorter
5. Output the summary directly, no prefix like "Summary:" needed`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: history},
		},
		Temperature: 0.3,
		MaxTokens:   300,
	})
	if err != nil {
		return "", fmt.Errorf("summarize chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	summary := strings.TrimSpace(resp.Choices[0].Message.Content)
	fmt.Printf("[Moonshot] Chat summary: %s\n", summary)
	return summary, nil
}
