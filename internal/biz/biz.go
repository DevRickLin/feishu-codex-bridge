package biz

import (
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// Usecases contains all usecases
type Usecases struct {
	Session      *usecase.SessionUsecase
	Context      *usecase.ContextBuilderUsecase
	Filter       *usecase.FilterUsecase
	Conversation *usecase.ConversationUsecase
}
