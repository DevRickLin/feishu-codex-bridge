package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropics/feishu-codex-bridge/internal/mcp"
)

// This MCP server communicates with the Bridge process via HTTP API.
// It provides Feishu tools to Codex and relays tool calls to the Bridge.

// Environment variable for Bridge API URL
var bridgeAPIURL = os.Getenv("BRIDGE_API_URL")

// MCP Protocol types
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

var handler *mcp.Handler

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Initialize HTTP client and handler
	if bridgeAPIURL == "" {
		bridgeAPIURL = "http://127.0.0.1:9876" // Default port
	}
	client := mcp.NewClient(bridgeAPIURL)
	handler = mcp.NewHandler(client)

	// Read from stdin, write to stdout (MCP stdio transport)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *MCPRequest) {
	switch req.Method {
	case "initialize":
		handleInitialize(req)
	case "tools/list":
		handleToolsList(req)
	case "tools/call":
		handleToolsCall(req)
	case "notifications/initialized":
		// Notification, no response needed
	default:
		sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func handleInitialize(req *MCPRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "feishu-mcp",
			"version": "1.0.0",
		},
	}
	sendResult(req.ID, result)
}

func handleToolsList(req *MCPRequest) {
	tools := mcp.GetToolDefinitions()

	// Convert to the format expected by MCP
	toolMaps := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		toolMaps[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
	}

	result := map[string]interface{}{
		"tools": toolMaps,
	}
	sendResult(req.ID, result)
}

func handleToolsCall(req *MCPRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	result, err := handler.HandleToolCall(params.Name, params.Arguments)
	if err != nil {
		sendToolResult(req.ID, false, err.Error())
		return
	}

	resultJSON, _ := json.Marshal(result)
	sendToolResult(req.ID, true, string(resultJSON))
}

func sendResult(id interface{}, result interface{}) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	sendResponse(resp)
}

func sendError(id interface{}, code int, message, data string) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	sendResponse(resp)
}

func sendToolResult(id interface{}, isSuccess bool, content string) {
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": content,
			},
		},
		"isError": !isSuccess,
	}
	sendResult(id, result)
}

func sendResponse(resp MCPResponse) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
