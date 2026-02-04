package data

import (
	"context"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/moonshot"
)

// TopicsProvider provides interest topics interface
type TopicsProvider interface {
	GetInterestTopics(ctx context.Context) ([]string, error)
}

// moonshotRepo implements the Moonshot filter repository
type moonshotRepo struct {
	client         *moonshot.Client
	botName        string
	topicsProvider TopicsProvider
}

// NewMoonshotRepo creates a Moonshot repository
func NewMoonshotRepo(client *moonshot.Client) repo.FilterRepo {
	if client == nil {
		return nil
	}
	return &moonshotRepo{client: client}
}

// NewMoonshotRepoWithBotName creates a Moonshot repository with bot name
func NewMoonshotRepoWithBotName(client *moonshot.Client, botName string) repo.FilterRepo {
	if client == nil {
		return nil
	}
	return &moonshotRepo{client: client, botName: botName}
}

// NewMoonshotRepoWithTopics creates a Moonshot repository with bot name and topics provider
func NewMoonshotRepoWithTopics(client *moonshot.Client, botName string, topicsProvider TopicsProvider) repo.FilterRepo {
	if client == nil {
		return nil
	}
	return &moonshotRepo{client: client, botName: botName, topicsProvider: topicsProvider}
}

// ShouldRespond determines if the bot should respond
func (r *moonshotRepo) ShouldRespond(ctx context.Context, message, history, strategy string) (bool, error) {
	// If botName is set and no custom strategy specified, use strategy with botName
	if r.botName != "" && strategy == "" {
		// Try to get interest topics
		var topics []string
		if r.topicsProvider != nil {
			topics, _ = r.topicsProvider.GetInterestTopics(ctx)
		}
		strategy = moonshot.GetListenStrategyWithTopics(r.botName, topics)
	}
	should, _ := r.client.ShouldRespondWithStrategy(message, history, strategy)
	return should, nil
}

// SummarizeHistory summarizes chat history
func (r *moonshotRepo) SummarizeHistory(ctx context.Context, history string) (string, error) {
	return r.client.SummarizeChatHistory(history)
}
