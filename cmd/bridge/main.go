package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/anthropics/feishu-codex-bridge/codex"
	"github.com/anthropics/feishu-codex-bridge/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
	"github.com/anthropics/feishu-codex-bridge/internal/conf"
	"github.com/anthropics/feishu-codex-bridge/internal/data"
	"github.com/anthropics/feishu-codex-bridge/internal/server"
	"github.com/anthropics/feishu-codex-bridge/internal/service"
	"github.com/anthropics/feishu-codex-bridge/ipc"
	"github.com/anthropics/feishu-codex-bridge/moonshot"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := conf.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Initialize clients
	feishuClient := feishu.NewClient(cfg.Feishu.AppID, cfg.Feishu.AppSecret)
	codexClient := codex.NewClient(cfg.Codex.WorkingDir, cfg.Codex.Model)

	// Start Codex client
	ctx := context.Background()
	if err := codexClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start Codex client: %v", err)
	}
	fmt.Println("[Bridge] Codex client started")

	var moonshotClient *moonshot.Client
	if cfg.Moonshot.APIKey != "" {
		moonshotClient = moonshot.NewClient(cfg.Moonshot.APIKey, cfg.Moonshot.Model)
		fmt.Println("[Bridge] Moonshot pre-filter enabled")
	}

	// Initialize repository layer
	repos, err := data.NewRepositories(feishuClient, codexClient, moonshotClient, cfg.Session.DBPath, cfg.Feishu.BotName)
	if err != nil {
		log.Fatalf("Failed to create repositories: %v", err)
	}

	fmt.Printf("[Bridge] Session DB: %s\n", cfg.Session.DBPath)

	// Initialize usecase layer
	sessionCfg := cfg.Session.ToSessionConfig()
	promptCfg := cfg.ToPromptConfig()

	contextUC := usecase.NewContextBuilderUsecase(repos.Message)
	sessionUC := usecase.NewSessionUsecase(repos.Session, repos.Codex, sessionCfg)
	filterUC := usecase.NewFilterUsecase(repos.Filter, repos.Message, contextUC)
	convUC := usecase.NewConversationUsecase(sessionUC, contextUC, repos.Codex, promptCfg)

	// Initialize service layer
	convSvc := service.NewConversationService(convUC, filterUC, repos.Message, repos.Codex)

	// Initialize Buffer usecase
	bufferCfg := usecase.DefaultBufferConfig()
	bufferUC := usecase.NewBufferUsecase(repos.Buffer, bufferCfg)

	// Initialize IPC handler
	dataDir := filepath.Dir(cfg.Session.DBPath)
	ipcHandler, err := ipc.NewHandler(dataDir, createIPCActionHandler(repos, bufferUC))
	if err != nil {
		log.Fatalf("Failed to create IPC handler: %v", err)
	}
	ipcHandler.Start(ctx)
	fmt.Println("[Bridge] IPC handler started")

	// Configure MCP server
	mcpPath, err := findMCPServerPath()
	if err != nil {
		fmt.Printf("[Bridge] Warning: MCP server not found: %v\n", err)
	} else {
		codexClient.SetMCPServer(mcpPath, ipcHandler.GetEnvVars())
		fmt.Printf("[Bridge] MCP server configured: %s\n", mcpPath)
	}

	// Initialize server
	// Pass codexRepo and filterUC to enable Codex smart digest + Moonshot filtering
	srv := server.NewFeishuServer(feishuClient, repos.Message, convSvc, bufferUC, repos.Codex, filterUC)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		srv.Stop()
		ipcHandler.Stop()
		codexClient.Stop()
		os.Exit(0)
	}()

	fmt.Println("Starting Feishu-Codex Bridge (Kratos style)...")
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// findMCPServerPath finds the feishu-mcp binary
func findMCPServerPath() (string, error) {
	// Try to find in common locations
	candidates := []string{
		"./feishu-mcp",
		"./cmd/feishu-mcp/feishu-mcp",
		"/usr/local/bin/feishu-mcp",
	}

	// Check GOPATH/bin
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidates = append(candidates, filepath.Join(gopath, "bin", "feishu-mcp"))
	}
	candidates = append(candidates, filepath.Join(os.Getenv("HOME"), "go", "bin", "feishu-mcp"))

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath, nil
		}
	}

	// Try to find via `which`
	if path, err := exec.LookPath("feishu-mcp"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("feishu-mcp not found")
}

// createIPCActionHandler creates the action handler for IPC requests
func createIPCActionHandler(repos *data.Repositories, bufferUC *usecase.BufferUsecase) ipc.ActionHandler {
	return func(chatID, msgID string, action string, args map[string]interface{}) (interface{}, error) {
		ctx := context.Background()

		switch action {
		case "get_chat_history":
			limit := 20
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}
			messages, err := repos.Message.GetChatHistory(ctx, chatID, limit)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"messages": messages,
				"source":   "feishu_api",
			}, nil

		// Buffer management actions
		case "add_to_whitelist":
			chatID, _ := args["chat_id"].(string)
			reason, _ := args["reason"].(string)
			return nil, bufferUC.AddToWhitelist(ctx, chatID, reason, "codex")

		case "remove_from_whitelist":
			chatID, _ := args["chat_id"].(string)
			return nil, bufferUC.RemoveFromWhitelist(ctx, chatID)

		case "list_whitelist":
			entries, err := bufferUC.GetWhitelist(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"entries": entries}, nil

		case "add_keyword":
			keyword, _ := args["keyword"].(string)
			priority := 1
			if p, ok := args["priority"].(float64); ok {
				priority = int(p)
			}
			return nil, bufferUC.AddKeyword(ctx, keyword, priority)

		case "remove_keyword":
			keyword, _ := args["keyword"].(string)
			return nil, bufferUC.RemoveKeyword(ctx, keyword)

		case "list_keywords":
			keywords, err := bufferUC.GetKeywords(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"keywords": keywords}, nil

		case "get_buffer_summary":
			summaries, err := bufferUC.GetBufferSummary(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"summaries": summaries}, nil

		case "get_buffered_messages":
			chatID, _ := args["chat_id"].(string)
			messages, err := bufferUC.GetUnprocessedMessages(ctx, chatID)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"messages": messages}, nil

		// Interest topic management
		case "add_interest_topic":
			topic, _ := args["topic"].(string)
			return nil, bufferUC.AddInterestTopic(ctx, topic, "")

		case "remove_interest_topic":
			topic, _ := args["topic"].(string)
			return nil, bufferUC.RemoveInterestTopic(ctx, topic)

		case "list_interest_topics":
			topics, err := bufferUC.GetInterestTopics(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"topics": topics}, nil

		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	}
}
