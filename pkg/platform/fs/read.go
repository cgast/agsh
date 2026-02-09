package fs

import (
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// ReadCommand implements fs:read — reads the contents of a file.
type ReadCommand struct{}

func (c *ReadCommand) Name() string        { return "fs:read" }
func (c *ReadCommand) Description() string { return "Read file contents" }
func (c *ReadCommand) Namespace() string   { return "fs" }

func (c *ReadCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"path": {Type: "string", Description: "File path to read"},
		},
		Required: []string{"path"},
	}
}

func (c *ReadCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"content": {Type: "string", Description: "File contents"},
		},
	}
}

func (c *ReadCommand) RequiredCredentials() []string { return nil }

func (c *ReadCommand) Execute(_ gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	filePath, err := extractFilePath(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:read: %w", err)
	}

	filePath, err = filepath.Abs(filePath)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:read: resolve path: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("fs:read: %w", err)
	}

	env := agshctx.NewEnvelope(string(data), "text/plain", "fs:read")
	env.Meta.Tags["path"] = filePath
	env.Meta.Tags["size"] = fmt.Sprintf("%d", len(data))
	return env, nil
}

// extractFilePath gets a file path from the input envelope.
// Supports string payload, map with "path" key, or FileEntry from fs:list.
func extractFilePath(input agshctx.Envelope) (string, error) {
	switch v := input.Payload.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("empty file path")
		}
		return v, nil
	case map[string]any:
		if p, ok := v["path"]; ok {
			if s, ok := p.(string); ok {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("cannot extract file path from payload type %T", input.Payload)
}
