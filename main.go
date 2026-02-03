package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/anthropics/feishu-codex-bridge/bridge"
	"github.com/joho/godotenv"
)

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Parse session config
	sessionIdleMin := 60 // default 60 minutes
	if val := os.Getenv("SESSION_IDLE_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			sessionIdleMin = parsed
		}
	}

	sessionResetHr := 4 // default 4 AM
	if val := os.Getenv("SESSION_RESET_HOUR"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			sessionResetHr = parsed
		}
	}

	// Session DB path
	sessionDBPath := os.Getenv("SESSION_DB_PATH")
	if sessionDBPath == "" {
		homeDir, _ := os.UserHomeDir()
		sessionDBPath = filepath.Join(homeDir, ".feishu-codex", "sessions.db")
	}

	config := bridge.Config{
		FeishuAppID:     os.Getenv("FEISHU_APP_ID"),
		FeishuAppSecret: os.Getenv("FEISHU_APP_SECRET"),
		WorkingDir:      os.Getenv("WORKING_DIR"),
		CodexModel:      os.Getenv("CODEX_MODEL"),
		SessionDBPath:   sessionDBPath,
		SessionIdleMin:  sessionIdleMin,
		SessionResetHr:  sessionResetHr,
		Debug:           os.Getenv("DEBUG") == "true",
	}

	if config.FeishuAppID == "" || config.FeishuAppSecret == "" {
		log.Fatal("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}

	if config.WorkingDir == "" {
		config.WorkingDir = "."
	}

	b, err := bridge.New(config)
	if err != nil {
		log.Fatalf("Failed to create bridge: %v", err)
	}

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		b.Stop()
		os.Exit(0)
	}()

	fmt.Println("Starting Feishu-Codex Bridge (ACP mode)...")
	if err := b.Start(); err != nil {
		log.Fatalf("Bridge error: %v", err)
	}
}
