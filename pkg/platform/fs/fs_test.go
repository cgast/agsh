package fs

import (
	gocontext "context"
	"os"
	"path/filepath"
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestListCommand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("bb"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	cmd := &ListCommand{}
	input := agshctx.NewEnvelope(dir, "text/plain", "test")

	env, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	files, ok := env.Payload.([]FileEntry)
	if !ok {
		t.Fatalf("expected []FileEntry payload, got %T", env.Payload)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(files))
	}

	// Should be sorted alphabetically.
	if files[0].Name != "a.txt" {
		t.Errorf("expected first file 'a.txt', got %s", files[0].Name)
	}
	if files[1].Name != "b.md" {
		t.Errorf("expected second file 'b.md', got %s", files[1].Name)
	}
	if files[2].Name != "subdir" {
		t.Errorf("expected third entry 'subdir', got %s", files[2].Name)
	}
	if !files[2].IsDir {
		t.Error("expected subdir to be a directory")
	}

	if env.Meta.ContentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", env.Meta.ContentType)
	}
	if env.Meta.Source != "fs:list" {
		t.Errorf("expected source fs:list, got %s", env.Meta.Source)
	}
}

func TestListCommandMapPayload(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	cmd := &ListCommand{}
	input := agshctx.NewEnvelope(map[string]any{"path": dir}, "application/json", "test")

	env, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	files := env.Payload.([]FileEntry)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestListCommandNilPayload(t *testing.T) {
	cmd := &ListCommand{}
	input := agshctx.NewEnvelope(nil, "text/plain", "test")

	// Should default to current directory without error.
	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
}

func TestListCommandNonexistentDir(t *testing.T) {
	cmd := &ListCommand{}
	input := agshctx.NewEnvelope("/nonexistent/path", "text/plain", "test")

	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestReadCommand(t *testing.T) {
	dir := t.TempDir()
	content := "hello, agsh!"
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte(content), 0644)

	cmd := &ReadCommand{}
	input := agshctx.NewEnvelope(path, "text/plain", "test")

	env, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	got, ok := env.Payload.(string)
	if !ok {
		t.Fatalf("expected string payload, got %T", env.Payload)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
	if env.Meta.Source != "fs:read" {
		t.Errorf("expected source fs:read, got %s", env.Meta.Source)
	}
}

func TestReadCommandMapPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("content"), 0644)

	cmd := &ReadCommand{}
	input := agshctx.NewEnvelope(map[string]any{"path": path}, "application/json", "test")

	env, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if env.Payload != "content" {
		t.Errorf("expected 'content', got %v", env.Payload)
	}
}

func TestReadCommandNonexistentFile(t *testing.T) {
	cmd := &ReadCommand{}
	input := agshctx.NewEnvelope("/nonexistent/file.txt", "text/plain", "test")

	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestWriteCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.md")
	content := "# Hello\n\nThis is a test."

	cmd := &WriteCommand{}
	input := agshctx.NewEnvelope(map[string]any{
		"path":    path,
		"content": content,
	}, "application/json", "test")

	env, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	result, ok := env.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", env.Payload)
	}

	if result["bytes_written"] != len(content) {
		t.Errorf("expected bytes_written=%d, got %v", len(content), result["bytes_written"])
	}

	// Verify file was written.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content mismatch: expected %q, got %q", content, string(data))
	}
}

func TestWriteCommandCreatesSubdirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "output.txt")

	cmd := &WriteCommand{}
	input := agshctx.NewEnvelope(map[string]any{
		"path":    path,
		"content": "nested content",
	}, "application/json", "test")

	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("expected 'nested content', got %q", string(data))
	}
}

func TestWriteCommandMissingPath(t *testing.T) {
	cmd := &WriteCommand{}
	input := agshctx.NewEnvelope(map[string]any{
		"content": "data",
	}, "application/json", "test")

	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestWriteCommandMissingContent(t *testing.T) {
	cmd := &WriteCommand{}
	input := agshctx.NewEnvelope(map[string]any{
		"path": "/tmp/test.txt",
	}, "application/json", "test")

	_, err := cmd.Execute(gocontext.Background(), input, nil)
	if err == nil {
		t.Error("expected error for missing content")
	}
}

func TestCommandIdentity(t *testing.T) {
	commands := []struct {
		cmd       interface{ Name() string; Namespace() string; Description() string }
		name      string
		namespace string
	}{
		{&ListCommand{}, "fs:list", "fs"},
		{&ReadCommand{}, "fs:read", "fs"},
		{&WriteCommand{}, "fs:write", "fs"},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Name() != tt.name {
				t.Errorf("expected name %s, got %s", tt.name, tt.cmd.Name())
			}
			if tt.cmd.Namespace() != tt.namespace {
				t.Errorf("expected namespace %s, got %s", tt.namespace, tt.cmd.Namespace())
			}
			if tt.cmd.Description() == "" {
				t.Error("expected non-empty description")
			}
		})
	}
}
