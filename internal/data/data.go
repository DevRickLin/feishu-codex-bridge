package data

import (
	"github.com/anthropics/feishu-codex-bridge/codex"
	"github.com/anthropics/feishu-codex-bridge/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/moonshot"
)

// Repositories contains all repositories
type Repositories struct {
	Message repo.MessageRepo
	Session repo.SessionRepo
	Codex   repo.CodexRepo
	Filter  repo.FilterRepo
	Buffer  repo.BufferRepo
}

// NewRepositories creates all repositories
func NewRepositories(
	feishuClient *feishu.Client,
	codexClient *codex.Client,
	moonshotClient *moonshot.Client,
	sessionDBPath string,
	botName string,
) (*Repositories, error) {
	sessionRepo, err := NewSessionRepo(sessionDBPath)
	if err != nil {
		return nil, err
	}

	// Buffer repository uses same database directory as Session
	bufferDBPath := sessionDBPath[:len(sessionDBPath)-len("sessions.db")] + "buffer.db"
	bufferRepo, err := NewBufferRepo(bufferDBPath)
	if err != nil {
		return nil, err
	}

	// bufferRepo implements TopicsProvider interface, passed to Moonshot for dynamic topic fetching
	return &Repositories{
		Message: NewFeishuRepo(feishuClient),
		Session: sessionRepo,
		Codex:   NewCodexRepo(codexClient),
		Filter:  NewMoonshotRepoWithTopics(moonshotClient, botName, bufferRepo),
		Buffer:  bufferRepo,
	}, nil
}
