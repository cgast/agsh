package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != "interactive" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "interactive")
	}
	if cfg.Approval.Mode != "plan" {
		t.Errorf("Approval.Mode = %q, want %q", cfg.Approval.Mode, "plan")
	}
	if !cfg.Verify.FailFast {
		t.Error("Verify.FailFast should be true by default")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
mode: agent
log_level: debug
approval:
  mode: never
  timeout: 60
verify:
  fail_fast: false
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Mode != "agent" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "agent")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.Approval.Mode != "never" {
		t.Errorf("Approval.Mode = %q, want %q", cfg.Approval.Mode, "never")
	}
	if cfg.Approval.Timeout != 60 {
		t.Errorf("Approval.Timeout = %d, want %d", cfg.Approval.Timeout, 60)
	}
	if cfg.Verify.FailFast {
		t.Error("Verify.FailFast should be false")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Mode != "interactive" {
		t.Errorf("Mode = %q, want default %q", cfg.Mode, "interactive")
	}
}

func TestLoadPlatformConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platforms.yaml")

	// Set env var for interpolation.
	t.Setenv("TEST_GH_TOKEN", "ghp_test123")

	yaml := `
github:
  token: "${TEST_GH_TOKEN}"
  default_owner: "testuser"
http:
  allowed_domains:
    - "api.github.com"
    - "httpbin.org"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadPlatformConfig(path)
	if err != nil {
		t.Fatalf("LoadPlatformConfig: %v", err)
	}

	if cfg.GitHub.Token != "ghp_test123" {
		t.Errorf("GitHub.Token = %q, want %q", cfg.GitHub.Token, "ghp_test123")
	}
	if cfg.GitHub.DefaultOwner != "testuser" {
		t.Errorf("GitHub.DefaultOwner = %q, want %q", cfg.GitHub.DefaultOwner, "testuser")
	}
	if len(cfg.HTTP.AllowedDomains) != 2 {
		t.Errorf("HTTP.AllowedDomains = %v, want 2 domains", cfg.HTTP.AllowedDomains)
	}
}

func TestLoadPlatformConfigMissing(t *testing.T) {
	cfg, err := LoadPlatformConfig("/nonexistent/path/platforms.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.GitHub.Token != "" {
		t.Errorf("GitHub.Token should be empty, got %q", cfg.GitHub.Token)
	}
}

func TestInterpolateEnvVars(t *testing.T) {
	t.Setenv("FOO", "bar")
	t.Setenv("NUM_123", "456")

	tests := []struct {
		input string
		want  string
	}{
		{"${FOO}", "bar"},
		{"prefix-${FOO}-suffix", "prefix-bar-suffix"},
		{"${UNSET_VAR}", "${UNSET_VAR}"}, // unresolved stays
		{"${FOO} and ${NUM_123}", "bar and 456"},
		{"no vars here", "no vars here"},
	}

	for _, tt := range tests {
		got := interpolateEnvVars(tt.input)
		if got != tt.want {
			t.Errorf("interpolateEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
