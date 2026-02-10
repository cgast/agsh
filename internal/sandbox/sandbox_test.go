package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		s, err := New(Config{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.allowedPaths) != 0 {
			t.Errorf("expected no allowed paths, got %d", len(s.allowedPaths))
		}
		if s.maxFileSize != 0 {
			t.Errorf("expected no max file size, got %d", s.maxFileSize)
		}
	})

	t.Run("with file size", func(t *testing.T) {
		s, err := New(Config{MaxFileSize: "10MB"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.maxFileSize != 10*1024*1024 {
			t.Errorf("expected 10MB = %d bytes, got %d", 10*1024*1024, s.maxFileSize)
		}
	})

	t.Run("invalid file size", func(t *testing.T) {
		_, err := New(Config{MaxFileSize: "notasize"})
		if err == nil {
			t.Fatal("expected error for invalid file size")
		}
	})
}

func TestCheckPath(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	deniedDir := filepath.Join(tmpDir, "denied")
	otherDir := filepath.Join(tmpDir, "other")
	os.MkdirAll(allowedDir, 0755)
	os.MkdirAll(deniedDir, 0755)
	os.MkdirAll(otherDir, 0755)

	s, err := New(Config{
		AllowedPaths: []string{allowedDir, deniedDir},
		DeniedPaths:  []string{deniedDir},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"allowed dir itself", allowedDir, false},
		{"file in allowed dir", filepath.Join(allowedDir, "file.txt"), false},
		{"nested in allowed dir", filepath.Join(allowedDir, "sub", "file.txt"), false},
		{"denied dir itself", deniedDir, true},
		{"file in denied dir", filepath.Join(deniedDir, "secret.txt"), true},
		{"path not in allowed list", otherDir, true},
		{"file in unlisted dir", filepath.Join(otherDir, "file.txt"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.CheckPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestCheckPath_NoAllowedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	deniedDir := filepath.Join(tmpDir, "denied")
	os.MkdirAll(deniedDir, 0755)

	s, err := New(Config{
		DeniedPaths: []string{deniedDir},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no allowed paths, all non-denied paths should be OK.
	if err := s.CheckPath(tmpDir); err != nil {
		t.Errorf("expected no error for non-denied path, got: %v", err)
	}

	// Denied path should still be blocked.
	if err := s.CheckPath(deniedDir); err == nil {
		t.Error("expected error for denied path")
	}
}

func TestCheckFileSize(t *testing.T) {
	s, err := New(Config{MaxFileSize: "1KB"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"zero bytes", 0, false},
		{"within limit", 512, false},
		{"exactly at limit", 1024, false},
		{"over limit", 1025, true},
		{"way over limit", 1024 * 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.CheckFileSize(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckFileSize(%d) error = %v, wantErr %v", tt.size, err, tt.wantErr)
			}
		})
	}
}

func TestCheckFileSize_NoLimit(t *testing.T) {
	s, err := New(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.CheckFileSize(1024 * 1024 * 1024); err != nil {
		t.Errorf("expected no error with no limit, got: %v", err)
	}
}

func TestParseFileSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"100", 100, false},
		{"100B", 100, false},
		{"1KB", 1024, false},
		{"10KB", 10240, false},
		{"1MB", 1024 * 1024, false},
		{"10MB", 10 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"0.5MB", 512 * 1024, false},
		{"  5MB  ", 5 * 1024 * 1024, false},
		{"1mb", 1024 * 1024, false}, // case-insensitive
		{"", 0, true},
		{"abc", 0, true},
		{"MB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseFileSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFileSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && result != tt.expected {
				t.Errorf("parseFileSize(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
