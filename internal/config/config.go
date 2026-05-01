package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Discord     DiscordConfig     `yaml:"discord"`
	LLM         LLMConfig         `yaml:"llm"`
	Workspace   string            `yaml:"workspace"`
	Agent       AgentConfig       `yaml:"agent"`
	Lua         LuaConfig         `yaml:"lua"`
	GitOps      GitOpsConfig      `yaml:"gitops"`
	Tools       ToolsConfig       `yaml:"tools"`
	SelfImprove SelfImproveConfig `yaml:"self_improve"`
	Browser     BrowserConfig     `yaml:"browser"`
	Memory      MemoryConfig      `yaml:"memory"`
	Scheduler   SchedulerConfig   `yaml:"scheduler"`
	Inference   InferenceConfig   `yaml:"inference"`
}

type DiscordConfig struct {
	Token           string        `yaml:"token"`
	Trigger         TriggerConfig `yaml:"trigger"`
	WatchedChannels []string      `yaml:"watched_channels"`
	DefaultChannel  string        `yaml:"default_channel"`
}

type TriggerConfig struct {
	Mentions       bool `yaml:"mentions"`
	DirectMessages bool `yaml:"direct_messages"`
}

type LLMConfig struct {
	Provider string `yaml:"provider"`
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	BaseURL  string `yaml:"base_url"`
}

type AgentConfig struct {
	InjectMode        string `yaml:"inject_mode"`
	BootstrapMaxChars int    `yaml:"bootstrap_max_chars"`
	TimeoutSeconds    int    `yaml:"timeout_seconds"`
}

type LuaConfig struct {
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	MemoryLimitMB  int      `yaml:"memory_limit_mb"`
	FilePaths      []string `yaml:"file_paths"`
}

type GitOpsConfig struct {
	AutoCommit          bool   `yaml:"auto_commit"`
	ErrorThreshold      int    `yaml:"error_threshold"`
	CommitMessagePrefix string `yaml:"commit_message_prefix"`
}

type ToolsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type SelfImproveConfig struct {
	Enabled           bool `yaml:"enabled"`
	AutoDetectGaps    bool `yaml:"auto_detect_gaps"`
	AutoCreateSkills  bool `yaml:"auto_create_skills"`
	ErrorThreshold    int  `yaml:"error_threshold"`
	EvaluationEnabled bool `yaml:"evaluation_enabled"`
}

type BrowserConfig struct {
	Headless       bool `yaml:"headless"`
	TimeoutSeconds int  `yaml:"timeout_seconds"`
}

type MemoryConfig struct {
	IndexEnabled    bool `yaml:"index_enabled"`
	DailyAutoCreate bool `yaml:"daily_auto_create"`
	GrepSearch      bool `yaml:"grep_search"`
}

// SchedulerConfig configures the scheduled task system
type SchedulerConfig struct {
	Enabled     bool         `yaml:"enabled"`
	HeartbeatMs int64        `yaml:"heartbeat_ms"` // Check interval in ms (default: 30min = 1800000)
	Timezone    string       `yaml:"timezone"`     // IANA timezone (default: "UTC")
	Tasks       []TaskConfig `yaml:"tasks"`        // Pre-defined scheduled tasks
}

// TaskConfig defines a single scheduled task
type TaskConfig struct {
	Name         string `yaml:"name"`           // Task name
	Schedule     string `yaml:"schedule"`       // Cron expression (e.g., "0 9 * * *")
	Message      string `yaml:"message"`        // Message to send to agent
	Enabled      bool   `yaml:"enabled"`        // Whether task is active
	MaxRunTimeMs int64  `yaml:"max_runtime_ms"` // Max runtime before force stop
}

// InferenceConfig configures the context inference system
type InferenceConfig struct {
	Enabled             bool    `yaml:"enabled"`
	Model               string  `yaml:"model"`               // Separate model for inference
	Provider            string  `yaml:"provider"`            // Can differ from execution
	MaxContextLen       int     `yaml:"max_context_len"`     // Max chars for retrieved context
	ConfidenceThreshold float64 `yaml:"confidence_treshold"` // Min confidence to retrieve
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist - return defaults
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Agent.InjectMode == "" {
		cfg.Agent.InjectMode = "every-turn"
	}
	if cfg.Agent.BootstrapMaxChars == 0 {
		cfg.Agent.BootstrapMaxChars = 20000
	}
	if cfg.Agent.TimeoutSeconds == 0 {
		cfg.Agent.TimeoutSeconds = 120
	}
	if cfg.Lua.TimeoutSeconds == 0 {
		cfg.Lua.TimeoutSeconds = 30
	}
	if cfg.Lua.MemoryLimitMB == 0 {
		cfg.Lua.MemoryLimitMB = 64
	}
	if cfg.GitOps.ErrorThreshold == 0 {
		cfg.GitOps.ErrorThreshold = 3
	}
	if cfg.Scheduler.HeartbeatMs == 0 {
		cfg.Scheduler.HeartbeatMs = 1800000
	}
	if cfg.Scheduler.Timezone == "" {
		cfg.Scheduler.Timezone = "UTC"
	}
	if cfg.Inference.MaxContextLen == 0 {
		cfg.Inference.MaxContextLen = 20000
	}
	if cfg.Inference.ConfidenceThreshold == 0 {
		cfg.Inference.ConfidenceThreshold = 0.3
	}

	// Environment variable overrides
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		cfg.LLM.BaseURL = baseURL
	}
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		cfg.LLM.Model = model
	}
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		cfg.LLM.Provider = provider
	}

	return &cfg, nil
}

func (c *Config) ExpandWorkspace() string {
	ws := c.Workspace
	if ws == "~/.naurvis/workspace" {
		home, _ := os.UserHomeDir()
		ws = home + "/.naurvis/workspace"
	}
	return ws
}
