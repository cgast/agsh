package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the runtime configuration from .agsh/config.yaml.
type Config struct {
	Mode      string          `yaml:"mode"`
	LogLevel  string          `yaml:"log_level"`
	Sandbox   SandboxConfig   `yaml:"sandbox"`
	Approval  ApprovalConfig  `yaml:"approval"`
	Verify    VerifyConfig    `yaml:"verify"`
	History   HistoryConfig   `yaml:"history"`
	Inspector InspectorConfig `yaml:"inspector"`
}

// InspectorConfig defines inspector GUI settings.
type InspectorConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// SandboxConfig defines filesystem restrictions.
type SandboxConfig struct {
	Workdir      string   `yaml:"workdir"`
	AllowedPaths []string `yaml:"allowed_paths"`
	DeniedPaths  []string `yaml:"denied_paths"`
	MaxFileSize  string   `yaml:"max_file_size"`
}

// ApprovalConfig defines how execution approval works.
type ApprovalConfig struct {
	Mode    string `yaml:"mode"`    // "always", "plan", "destructive", "never"
	Timeout int    `yaml:"timeout"` // seconds
}

// VerifyConfig defines verification defaults.
type VerifyConfig struct {
	FailFast         bool   `yaml:"fail_fast"`
	LLMJudgeEndpoint string `yaml:"llm_judge_endpoint"`
	LLMJudgeModel    string `yaml:"llm_judge_model"`
}

// HistoryConfig defines execution history settings.
type HistoryConfig struct {
	MaxEntries int  `yaml:"max_entries"`
	Persist    bool `yaml:"persist"`
}

// PlatformConfig represents platform credentials from .agsh/platforms.yaml.
type PlatformConfig struct {
	GitHub GitHubConfig `yaml:"github"`
	HTTP   HTTPConfig   `yaml:"http"`
}

// GitHubConfig holds GitHub platform settings.
type GitHubConfig struct {
	Token        string `yaml:"token"`
	DefaultOwner string `yaml:"default_owner"`
}

// HTTPConfig holds HTTP platform settings.
type HTTPConfig struct {
	AllowedDomains []string `yaml:"allowed_domains"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:     "interactive",
		LogLevel: "info",
		Sandbox: SandboxConfig{
			Workdir:      "/workspace",
			AllowedPaths: []string{"/workspace", "/tmp"},
			DeniedPaths:  []string{"/etc", "/usr"},
			MaxFileSize:  "10MB",
		},
		Approval: ApprovalConfig{
			Mode:    "plan",
			Timeout: 300,
		},
		Verify: VerifyConfig{
			FailFast: true,
		},
		History: HistoryConfig{
			MaxEntries: 10000,
			Persist:    true,
		},
	}
}

// LoadConfig reads and parses a runtime config YAML file.
// Returns default config if the file doesn't exist.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// LoadPlatformConfig reads and parses a platform credentials YAML file.
// Performs environment variable interpolation on string values.
func LoadPlatformConfig(path string) (PlatformConfig, error) {
	var cfg PlatformConfig

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read platform config %s: %w", path, err)
	}

	// Interpolate environment variables before parsing.
	interpolated := interpolateEnvVars(string(data))

	if err := yaml.Unmarshal([]byte(interpolated), &cfg); err != nil {
		return cfg, fmt.Errorf("parse platform config %s: %w", path, err)
	}

	return cfg, nil
}

// envVarPattern matches ${VAR_NAME} patterns.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolateEnvVars replaces ${VAR_NAME} patterns with environment variable values.
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match // Leave unresolved if not set.
	})
}
