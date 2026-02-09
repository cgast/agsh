package platform

import (
	gocontext "context"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// PlatformCommand defines the interface that all platform commands implement.
type PlatformCommand interface {
	// Identity
	Name() string
	Description() string
	Namespace() string

	// Schema
	InputSchema() Schema
	OutputSchema() Schema

	// Execution
	Execute(ctx gocontext.Context, input agshctx.Envelope, store agshctx.ContextStore) (agshctx.Envelope, error)

	// Auth
	RequiredCredentials() []string
}

// Schema describes the expected input or output shape of a command.
type Schema struct {
	Type       string                 `json:"type"`
	Properties map[string]SchemaField `json:"properties"`
	Required   []string               `json:"required"`
}

// SchemaField describes a single field within a schema.
type SchemaField struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
