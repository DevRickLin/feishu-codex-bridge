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

	"github.com/anthropics/feishu-codex-bridge/internal/api"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
	"github.com/anthropics/feishu-codex-bridge/internal/conf"
	"github.com/anthropics/feishu-codex-bridge/internal/data"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/acp"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/infra/openai"
	"github.com/anthropics/feishu-codex-bridge/internal/server"
	"github.com/anthropics/feishu-codex-bridge/internal/service"
	"github.com/joho/godotenv"
)

const defaultAPIPort = 9876

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
	codexClient := acp.NewClient(cfg.Codex.WorkingDir, cfg.Codex.Model)

	// Start Codex client
	ctx := context.Background()
	if err := codexClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start Codex client: %v", err)
	}
	fmt.Println("[Bridge] Codex client started")

	var moonshotClient *openai.Client
	if cfg.Moonshot.APIKey != "" {
		moonshotClient = openai.NewClient(cfg.Moonshot.APIKey, cfg.Moonshot.Model)
		fmt.Println("[Bridge] Moonshot pre-filter enabled")
	}

	// Initialize repository layer
	repos, err := data.NewRepositories(feishuClient, codexClient, moonshotClient, cfg.Session.DBPath, cfg.Feishu.BotName, cfg.Prompts)
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

	// Initialize HTTP API server for feishu-mcp
	apiServer := api.NewServer(repos.Message, bufferUC, defaultAPIPort)
	go func() {
		if err := apiServer.Start(); err != nil {
			fmt.Printf("[Bridge] API server error: %v\n", err)
		}
	}()
	fmt.Printf("[Bridge] HTTP API server started on port %d\n", defaultAPIPort)

	// Configure MCP server
	mcpPath, err := findMCPServerPath()
	if err != nil {
		fmt.Printf("[Bridge] Warning: MCP server not found: %v\n", err)
	} else {
		// Pass Bridge API URL to MCP server via environment variable
		mcpEnvVars := map[string]string{
			"BRIDGE_API_URL": fmt.Sprintf("http://127.0.0.1:%d", defaultAPIPort),
		}
		codexClient.SetMCPServer(mcpPath, mcpEnvVars)
		fmt.Printf("[Bridge] MCP server configured: %s\n", mcpPath)
	}

	// Initialize server
	// Pass codexRepo and filterUC to enable Codex smart digest + Moonshot filtering
	srv := server.NewFeishuServer(feishuClient, repos.Message, convSvc, bufferUC, repos.Codex, filterUC, apiServer)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		srv.Stop()
		apiServer.Stop()
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
