package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IPCRequest represents a request from the MCP server
type IPCRequest struct {
	Action    string                 `json:"action"`
	ChatID    string                 `json:"chat_id"`
	MessageID string                 `json:"message_id"`
	Arguments map[string]interface{} `json:"arguments"`
}

// IPCResponse represents a response to the MCP server
type IPCResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ChatContext represents the context for the MCP server
type ChatContext struct {
	ChatID     string          `json:"chat_id"`
	ChatType   string          `json:"chat_type"`
	MessageID  string          `json:"message_id"`
	Members    []Member        `json:"members"`
	RecentMsgs []RecentMessage `json:"recent_msgs"`
}

// Member represents a chat member
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecentMessage represents a recent message
type RecentMessage struct {
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// ActionHandler is the callback for handling IPC actions
// Returns (data, error) - data is included in response if not nil
type ActionHandler func(chatID, msgID string, action string, args map[string]interface{}) (interface{}, error)

// Handler manages IPC communication with the MCP server
type Handler struct {
	ipcDir        string
	requestFile   string
	responseFile  string
	contextFile   string
	actionHandler ActionHandler
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewHandler creates a new IPC handler
func NewHandler(baseDir string, handler ActionHandler) (*Handler, error) {
	ipcDir := filepath.Join(baseDir, "ipc")
	if err := os.MkdirAll(ipcDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create IPC directory: %w", err)
	}

	return &Handler{
		ipcDir:        ipcDir,
		requestFile:   filepath.Join(ipcDir, "request.json"),
		responseFile:  filepath.Join(ipcDir, "response.json"),
		contextFile:   filepath.Join(ipcDir, "context.json"),
		actionHandler: handler,
	}, nil
}

// GetEnvVars returns environment variables for the MCP server
func (h *Handler) GetEnvVars() map[string]string {
	return map[string]string{
		"FEISHU_MCP_REQUEST_FILE":  h.requestFile,
		"FEISHU_MCP_RESPONSE_FILE": h.responseFile,
		"FEISHU_MCP_CONTEXT_FILE":  h.contextFile,
	}
}

// UpdateContext writes the current chat context for the MCP server
func (h *Handler) UpdateContext(ctx *ChatContext) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(h.contextFile, data, 0644)
}

// Start begins polling for IPC requests
func (h *Handler) Start(ctx context.Context) {
	h.ctx, h.cancel = context.WithCancel(ctx)

	// Clear any stale files
	os.Remove(h.requestFile)
	os.Remove(h.responseFile)

	h.wg.Add(1)
	go h.pollLoop()
}

// Stop stops the IPC handler
func (h *Handler) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
}

func (h *Handler) pollLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.processRequest()
		}
	}
}

func (h *Handler) processRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if request file exists
	data, err := os.ReadFile(h.requestFile)
	if err != nil || len(data) == 0 {
		return
	}

	// Clear the request file immediately
	os.Remove(h.requestFile)

	var req IPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		h.writeResponse(IPCResponse{Success: false, Error: "invalid request"})
		return
	}

	// Process the action
	var resp IPCResponse
	if h.actionHandler != nil {
		data, err := h.actionHandler(req.ChatID, req.MessageID, req.Action, req.Arguments)
		if err != nil {
			resp = IPCResponse{Success: false, Error: err.Error()}
		} else {
			resp = IPCResponse{Success: true, Data: data}
		}
	} else {
		resp = IPCResponse{Success: false, Error: "no handler configured"}
	}

	h.writeResponse(resp)
}

func (h *Handler) writeResponse(resp IPCResponse) {
	data, _ := json.Marshal(resp)
	os.WriteFile(h.responseFile, data, 0644)
}

// GetIPCDir returns the IPC directory path
func (h *Handler) GetIPCDir() string {
	return h.ipcDir
}
