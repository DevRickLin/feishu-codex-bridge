package conf

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
)

// Config represents application configuration
type Config struct {
	// Feishu configuration
	Feishu FeishuConfig

	// Codex configuration
	Codex CodexConfig

	// Moonshot configuration (optional)
	Moonshot MoonshotConfig

	// Session configuration
	Session SessionConfig

	// Prompt configuration (legacy, kept for backward compatibility)
	Prompt PromptConfigValues

	// Prompts configuration (loaded from YAML)
	Prompts *PromptsConfig

	// MCP configuration
	MCP MCPConfig

	// Debug mode
	Debug bool
}

// PromptConfigValues contains prompt-related configuration values
type PromptConfigValues struct {
	MaxHistoryCount   int // Max number of history messages to keep
	MaxHistoryMinutes int // Max minutes of history messages to keep
}

// FeishuConfig contains Feishu configuration
type FeishuConfig struct {
	AppID     string
	AppSecret string
	BotName   string // Bot name, used for Moonshot filter
}

// CodexConfig contains Codex configuration
type CodexConfig struct {
	WorkingDir string
	Model      string
}

// MoonshotConfig contains Moonshot configuration
type MoonshotConfig struct {
	APIKey string
	Model  string
}

// SessionConfig contains session configuration
type SessionConfig struct {
	DBPath      string
	IdleMinutes int
	ResetHour   int
}

// MCPConfig contains MCP configuration
type MCPConfig struct {
	ServerPath string
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	// Session DB path
	sessionDBPath := os.Getenv("SESSION_DB_PATH")
	if sessionDBPath == "" {
		homeDir, _ := os.UserHomeDir()
		sessionDBPath = filepath.Join(homeDir, ".feishu-codex", "sessions.db")
	}

	// Idle timeout
	sessionIdleMin := 60
	if val := os.Getenv("SESSION_IDLE_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			sessionIdleMin = parsed
		}
	}

	// Daily reset hour
	sessionResetHr := 4
	if val := os.Getenv("SESSION_RESET_HOUR"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			sessionResetHr = parsed
		}
	}

	// Prompt history truncation configuration
	maxHistoryCount := 15 // Default max 15 messages
	if val := os.Getenv("MAX_HISTORY_COUNT"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			maxHistoryCount = parsed
		}
	}

	maxHistoryMinutes := 120 // Default 2 hours
	if val := os.Getenv("MAX_HISTORY_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			maxHistoryMinutes = parsed
		}
	}

	// MCP server path
	mcpServerPath := os.Getenv("MCP_SERVER_PATH")
	if mcpServerPath == "" {
		execPath, _ := os.Executable()
		mcpServerPath = filepath.Join(filepath.Dir(execPath), "feishu-mcp")
		if _, err := os.Stat(mcpServerPath); os.IsNotExist(err) {
			mcpServerPath = "./feishu-mcp"
			if _, err := os.Stat(mcpServerPath); os.IsNotExist(err) {
				mcpServerPath = ""
			}
		}
	}

	// Working directory
	workingDir := os.Getenv("WORKING_DIR")
	if workingDir == "" {
		workingDir = "."
	}

	// Load prompts from YAML
	promptsConfigPath := os.Getenv("PROMPTS_CONFIG_PATH")
	promptsConfig, _ := LoadPromptsConfig(promptsConfigPath)

	// Override history settings from env if specified
	if maxHistoryCount != 15 {
		promptsConfig.History.MaxCount = maxHistoryCount
	}
	if maxHistoryMinutes != 120 {
		promptsConfig.History.MaxMinutes = maxHistoryMinutes
	}

	return &Config{
		Feishu: FeishuConfig{
			AppID:     os.Getenv("FEISHU_APP_ID"),
			AppSecret: os.Getenv("FEISHU_APP_SECRET"),
			BotName:   os.Getenv("BOT_NAME"),
		},
		Codex: CodexConfig{
			WorkingDir: workingDir,
			Model:      os.Getenv("CODEX_MODEL"),
		},
		Moonshot: MoonshotConfig{
			APIKey: os.Getenv("MOONSHOT_API_KEY"),
			Model:  os.Getenv("MOONSHOT_MODEL"),
		},
		Session: SessionConfig{
			DBPath:      sessionDBPath,
			IdleMinutes: sessionIdleMin,
			ResetHour:   sessionResetHr,
		},
		Prompt: PromptConfigValues{
			MaxHistoryCount:   promptsConfig.History.MaxCount,
			MaxHistoryMinutes: promptsConfig.History.MaxMinutes,
		},
		Prompts: promptsConfig,
		MCP: MCPConfig{
			ServerPath: mcpServerPath,
		},
		Debug: os.Getenv("DEBUG") == "true",
	}
}

// ToSessionConfig converts to domain session configuration
func (c *SessionConfig) ToSessionConfig() domain.SessionConfig {
	return domain.SessionConfig{
		IdleTimeout: time.Duration(c.IdleMinutes) * time.Minute,
		ResetHour:   c.ResetHour,
	}
}

// ToPromptConfig converts to prompt configuration
func (c *Config) ToPromptConfig() usecase.PromptConfig {
	if c.Prompts == nil {
		cfg := usecase.DefaultPromptConfig
		cfg.MaxHistoryCount = c.Prompt.MaxHistoryCount
		cfg.MaxHistoryMinutes = c.Prompt.MaxHistoryMinutes
		return cfg
	}

	return usecase.PromptConfig{
		SystemPrompt:        c.Prompts.Codex.SystemPrompt,
		HistoryMarker:       c.Prompts.Codex.HistoryMarker,
		CurrentMarker:       c.Prompts.Codex.CurrentMarker,
		MemberListHeader:    c.Prompts.Codex.MemberListHeader,
		ChatContextTemplate: c.Prompts.Codex.ChatContextTemplate,
		MaxHistoryCount:     c.Prompts.History.MaxCount,
		MaxHistoryMinutes:   c.Prompts.History.MaxMinutes,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Feishu.AppID == "" || c.Feishu.AppSecret == "" {
		return &ConfigError{Field: "FEISHU_APP_ID/FEISHU_APP_SECRET", Message: "required"}
	}
	return nil
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Field + ": " + e.Message
}
