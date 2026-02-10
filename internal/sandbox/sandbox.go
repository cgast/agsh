package sandbox

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Sandbox enforces filesystem restrictions based on allowed/denied paths
// and file size limits. It is used by platform commands to validate
// operations before executing them.
type Sandbox struct {
	allowedPaths []string
	deniedPaths  []string
	maxFileSize  int64 // bytes, 0 means unlimited
}

// Config holds the sandbox configuration.
type Config struct {
	AllowedPaths []string
	DeniedPaths  []string
	MaxFileSize  string // e.g. "10MB", "1GB", "500KB"
}

// New creates a Sandbox from the given configuration.
// Allowed and denied paths are resolved to absolute paths.
func New(cfg Config) (*Sandbox, error) {
	s := &Sandbox{}

	for _, p := range cfg.AllowedPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve allowed path %q: %w", p, err)
		}
		s.allowedPaths = append(s.allowedPaths, abs)
	}

	for _, p := range cfg.DeniedPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve denied path %q: %w", p, err)
		}
		s.deniedPaths = append(s.deniedPaths, abs)
	}

	if cfg.MaxFileSize != "" {
		size, err := parseFileSize(cfg.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("sandbox: parse max_file_size %q: %w", cfg.MaxFileSize, err)
		}
		s.maxFileSize = size
	}

	return s, nil
}

// CheckPath validates that the given path is allowed by the sandbox.
// The path is resolved to an absolute path before checking.
// Returns nil if the path is allowed, or an error describing why it's denied.
func (s *Sandbox) CheckPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sandbox: resolve path %q: %w", path, err)
	}

	// Check denied paths first (deny takes precedence).
	for _, denied := range s.deniedPaths {
		if abs == denied || strings.HasPrefix(abs, denied+string(filepath.Separator)) {
			return fmt.Errorf("sandbox: path %q is under denied path %q", abs, denied)
		}
	}

	// If no allowed paths are configured, all non-denied paths are allowed.
	if len(s.allowedPaths) == 0 {
		return nil
	}

	// Check if path is under an allowed path.
	for _, allowed := range s.allowedPaths {
		if abs == allowed || strings.HasPrefix(abs, allowed+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("sandbox: path %q is not under any allowed path %v", abs, s.allowedPaths)
}

// CheckFileSize validates that the given size in bytes does not exceed
// the sandbox's maximum file size. Returns nil if the size is within limits
// or if no limit is configured.
func (s *Sandbox) CheckFileSize(size int64) error {
	if s.maxFileSize <= 0 {
		return nil
	}
	if size > s.maxFileSize {
		return fmt.Errorf("sandbox: file size %d bytes exceeds maximum %d bytes (%s)",
			size, s.maxFileSize, formatFileSize(s.maxFileSize))
	}
	return nil
}

// MaxFileSize returns the configured maximum file size in bytes.
// Returns 0 if no limit is configured.
func (s *Sandbox) MaxFileSize() int64 {
	return s.maxFileSize
}

// AllowedPaths returns the list of allowed absolute paths.
func (s *Sandbox) AllowedPaths() []string {
	return s.allowedPaths
}

// DeniedPaths returns the list of denied absolute paths.
func (s *Sandbox) DeniedPaths() []string {
	return s.deniedPaths
}

// parseFileSize parses a human-readable file size string into bytes.
// Supported suffixes: B, KB, MB, GB, TB (case-insensitive).
func parseFileSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	suffixes := []struct {
		suffix     string
		multiplier int64
	}{
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}

	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, sf.suffix))
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number %q", numStr)
			}
			return int64(n * float64(sf.multiplier)), nil
		}
	}

	// No suffix — assume bytes.
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid file size %q", s)
	}
	return n, nil
}

// formatFileSize formats bytes into a human-readable string.
func formatFileSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1fTB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
