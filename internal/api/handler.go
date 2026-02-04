package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// Server provides HTTP API for feishu-mcp to call back into Bridge
type Server struct {
	messageRepo repo.MessageRepo
	bufferUC    *usecase.BufferUsecase

	// Current chat context (updated when processing messages)
	currentContext *ChatContext
	contextMu      sync.RWMutex

	server *http.Server
	port   int
}

// ChatContext holds the current chat context for MCP tools
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

// NewServer creates a new API server
func NewServer(messageRepo repo.MessageRepo, bufferUC *usecase.BufferUsecase, port int) *Server {
	return &Server{
		messageRepo:    messageRepo,
		bufferUC:       bufferUC,
		currentContext: &ChatContext{},
		port:           port,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Chat operations
	mux.HandleFunc("/api/chat/", s.handleChat)

	// Whitelist management
	mux.HandleFunc("/api/whitelist", s.handleWhitelist)
	mux.HandleFunc("/api/whitelist/", s.handleWhitelistItem)

	// Keyword management
	mux.HandleFunc("/api/keywords", s.handleKeywords)
	mux.HandleFunc("/api/keywords/", s.handleKeywordItem)

	// Buffer management
	mux.HandleFunc("/api/buffer/summary", s.handleBufferSummary)
	mux.HandleFunc("/api/buffer/", s.handleBufferMessages)

	// Interest topics
	mux.HandleFunc("/api/topics", s.handleTopics)
	mux.HandleFunc("/api/topics/", s.handleTopicItem)

	// Context
	mux.HandleFunc("/api/context", s.handleContext)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}

	fmt.Printf("[API] Starting HTTP server on port %d\n", s.port)
	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

// GetPort returns the server port
func (s *Server) GetPort() int {
	return s.port
}

// SetContext updates the current chat context
func (s *Server) SetContext(ctx *ChatContext) {
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	s.currentContext = ctx
}

// GetContext returns the current chat context
func (s *Server) GetContext() *ChatContext {
	s.contextMu.RLock()
	defer s.contextMu.RUnlock()
	return s.currentContext
}

// ============ Chat Handlers ============

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/chat/{chat_id}/members or /api/chat/{chat_id}/history
	path := strings.TrimPrefix(r.URL.Path, "/api/chat/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	chatID := parts[0]
	action := parts[1]

	switch action {
	case "members":
		s.handleChatMembers(w, r, chatID)
	case "history":
		s.handleChatHistory(w, r, chatID)
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

func (s *Server) handleChatMembers(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	members, err := s.messageRepo.GetChatMembers(r.Context(), chatID)
	if err != nil {
		s.writeError(w, err)
		return
	}

	result := make([]Member, len(members))
	for i, m := range members {
		result[i] = Member{ID: m.UserID, Name: m.Name}
	}

	s.writeJSON(w, map[string]interface{}{"members": result})
}

func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	messages, err := s.messageRepo.GetChatHistory(r.Context(), chatID, limit)
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeJSON(w, map[string]interface{}{"messages": messages, "source": "feishu_api"})
}

// ============ Whitelist Handlers ============

func (s *Server) handleWhitelist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		entries, err := s.bufferUC.GetWhitelist(ctx)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"entries": entries})

	case http.MethodPost:
		var req struct {
			ChatID string `json:"chat_id"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ChatID == "" {
			req.ChatID = s.GetContext().ChatID
		}
		if req.ChatID == "" {
			http.Error(w, "chat_id is required", http.StatusBadRequest)
			return
		}
		if err := s.bufferUC.AddToWhitelist(ctx, req.ChatID, req.Reason, "mcp"); err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"success": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWhitelistItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chatID := strings.TrimPrefix(r.URL.Path, "/api/whitelist/")
	if chatID == "" {
		chatID = s.GetContext().ChatID
	}
	if chatID == "" {
		http.Error(w, "chat_id is required", http.StatusBadRequest)
		return
	}

	if err := s.bufferUC.RemoveFromWhitelist(r.Context(), chatID); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, map[string]interface{}{"success": true})
}

// ============ Keyword Handlers ============

func (s *Server) handleKeywords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		keywords, err := s.bufferUC.GetKeywords(ctx)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"keywords": keywords})

	case http.MethodPost:
		var req struct {
			Keyword  string `json:"keyword"`
			Priority int    `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Keyword == "" {
			http.Error(w, "keyword is required", http.StatusBadRequest)
			return
		}
		if req.Priority == 0 {
			req.Priority = 1
		}
		if err := s.bufferUC.AddKeyword(ctx, req.Keyword, req.Priority); err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"success": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleKeywordItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyword := strings.TrimPrefix(r.URL.Path, "/api/keywords/")
	if keyword == "" {
		http.Error(w, "keyword is required", http.StatusBadRequest)
		return
	}

	if err := s.bufferUC.RemoveKeyword(r.Context(), keyword); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, map[string]interface{}{"success": true})
}

// ============ Buffer Handlers ============

func (s *Server) handleBufferSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	summaries, err := s.bufferUC.GetBufferSummary(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, map[string]interface{}{"summaries": summaries})
}

func (s *Server) handleBufferMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/buffer/{chat_id}/messages
	path := strings.TrimPrefix(r.URL.Path, "/api/buffer/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "messages" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	chatID := parts[0]
	if chatID == "" {
		chatID = s.GetContext().ChatID
	}

	messages, err := s.bufferUC.GetUnprocessedMessages(r.Context(), chatID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, map[string]interface{}{"messages": messages})
}

// ============ Topic Handlers ============

func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		topics, err := s.bufferUC.GetInterestTopics(ctx)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"topics": topics})

	case http.MethodPost:
		var req struct {
			Topic string `json:"topic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Topic == "" {
			http.Error(w, "topic is required", http.StatusBadRequest)
			return
		}
		if err := s.bufferUC.AddInterestTopic(ctx, req.Topic, ""); err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, map[string]interface{}{"success": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTopicItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topic := strings.TrimPrefix(r.URL.Path, "/api/topics/")
	if topic == "" {
		http.Error(w, "topic is required", http.StatusBadRequest)
		return
	}

	if err := s.bufferUC.RemoveInterestTopic(r.Context(), topic); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, map[string]interface{}{"success": true})
}

// ============ Context Handler ============

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, s.GetContext())
}

// ============ Helpers ============

func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ConvertMembers converts domain.Member to api.Member
func ConvertMembers(members []domain.Member) []Member {
	result := make([]Member, len(members))
	for i, m := range members {
		result[i] = Member{ID: m.UserID, Name: m.Name}
	}
	return result
}
