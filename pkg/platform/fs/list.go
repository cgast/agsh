package fs

import (
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// ListCommand implements fs:list — lists files in a directory.
type ListCommand struct{}

func (c *ListCommand) Name() string        { return "fs:list" }
func (c *ListCommand) Description() string { return "List files in a directory" }
func (c *ListCommand) Namespace() string   { return "fs" }

func (c *ListCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"path": {Type: "string", Description: "Directory path to list"},
		},
		Required: []string{"path"},
	}
}

func (c *ListCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"files": {Type: "array", Description: "List of file entries"},
		},
	}
}

func (c *ListCommand) RequiredCredentials() []string { return nil }

// FileEntry represents a single file in a directory listing.
type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

func (c *ListCommand) Execute(_ gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	dir, err := extractPath(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:list: %w", err)
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:list: resolve path: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:list: read dir: %w", err)
	}

	files := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileEntry{
			Name:  entry.Name(),
			Path:  filepath.Join(dir, entry.Name()),
			Size:  info.Size(),
			IsDir: entry.IsDir(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	env := agshctx.NewEnvelope(files, "application/json", "fs:list")
	env.Meta.Tags["dir"] = dir
	env.Meta.Tags["count"] = fmt.Sprintf("%d", len(files))
	return env, nil
}

// extractPath gets the directory path from the input envelope.
// Supports string payload (path directly), or map with "path" key,
// or falls back to args-style.
func extractPath(input agshctx.Envelope) (string, error) {
	switch v := input.Payload.(type) {
	case string:
		if v == "" {
			return ".", nil
		}
		return v, nil
	case map[string]any:
		if p, ok := v["path"]; ok {
			if s, ok := p.(string); ok {
				return s, nil
			}
		}
	case nil:
		return ".", nil
	}
	return "", fmt.Errorf("cannot extract path from payload type %T", input.Payload)
}
