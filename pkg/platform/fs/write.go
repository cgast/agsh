package fs

import (
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// WriteCommand implements fs:write — writes content to a file.
type WriteCommand struct{}

func (c *WriteCommand) Name() string        { return "fs:write" }
func (c *WriteCommand) Description() string { return "Write content to a file" }
func (c *WriteCommand) Namespace() string   { return "fs" }

func (c *WriteCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"path":    {Type: "string", Description: "File path to write"},
			"content": {Type: "string", Description: "Content to write"},
		},
		Required: []string{"path", "content"},
	}
}

func (c *WriteCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"path":         {Type: "string", Description: "Written file path"},
			"bytes_written": {Type: "integer", Description: "Number of bytes written"},
		},
	}
}

func (c *WriteCommand) RequiredCredentials() []string { return nil }

func (c *WriteCommand) Execute(_ gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	filePath, content, err := extractWriteParams(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:write: %w", err)
	}

	filePath, err = filepath.Abs(filePath)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:write: resolve path: %w", err)
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:write: create dir: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:write: %w", err)
	}

	result := map[string]any{
		"path":          filePath,
		"bytes_written": len(content),
	}
	env := agshctx.NewEnvelope(result, "application/json", "fs:write")
	env.Meta.Tags["path"] = filePath
	return env, nil
}

// extractWriteParams gets the file path and content from the input envelope.
func extractWriteParams(input agshctx.Envelope) (string, string, error) {
	switch v := input.Payload.(type) {
	case map[string]any:
		path, ok := v["path"]
		if !ok {
			return "", "", fmt.Errorf("missing 'path' in payload")
		}
		pathStr, ok := path.(string)
		if !ok {
			return "", "", fmt.Errorf("'path' must be a string")
		}
		content, ok := v["content"]
		if !ok {
			return "", "", fmt.Errorf("missing 'content' in payload")
		}
		contentStr, ok := content.(string)
		if !ok {
			return "", "", fmt.Errorf("'content' must be a string")
		}
		return pathStr, contentStr, nil
	}
	return "", "", fmt.Errorf("fs:write requires map payload with 'path' and 'content' keys, got %T", input.Payload)
}
