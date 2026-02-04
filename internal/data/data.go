package data

import (
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/conf"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/acp"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/openai"
)

// Repositories contains all repositories
type Repositories struct {
	Message repo.MessageRepo
	Session repo.SessionRepo
	Codex   repo.CodexRepo
	Filter  repo.FilterRepo
	Buffer  repo.BufferRepo
	Memory  repo.MemoryRepo
}

// NewRepositories creates all repositories
func NewRepositories(
	feishuClient *feishu.Client,
	codexClient *acp.Client,
	moonshotClient *openai.Client,
	sessionDBPath string,
	botName string,
	promptsConfig *conf.PromptsConfig,
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

	// Memory repository for persistent memory storage
	memoryDBPath := sessionDBPath[:len(sessionDBPath)-len("sessions.db")] + "memory.db"
	memoryRepo, err := NewMemoryRepo(memoryDBPath)
	if err != nil {
		return nil, err
	}

	// bufferRepo implements TopicsProvider interface, passed to Moonshot for dynamic topic fetching
	return &Repositories{
		Message: NewFeishuRepo(feishuClient),
		Session: sessionRepo,
		Codex:   NewCodexRepo(codexClient),
		Filter:  NewMoonshotRepoWithConfig(moonshotClient, botName, bufferRepo, promptsConfig),
		Buffer:  bufferRepo,
		Memory:  memoryRepo,
	}, nil
}
