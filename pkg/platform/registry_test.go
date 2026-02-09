package platform

import (
	gocontext "context"
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// mockCommand is a test implementation of PlatformCommand.
type mockCommand struct {
	name      string
	namespace string
	desc      string
}

func (m *mockCommand) Name() string        { return m.name }
func (m *mockCommand) Description() string { return m.desc }
func (m *mockCommand) Namespace() string   { return m.namespace }
func (m *mockCommand) InputSchema() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaField{}}
}
func (m *mockCommand) OutputSchema() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaField{}}
}
func (m *mockCommand) Execute(_ gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	return input, nil
}
func (m *mockCommand) RequiredCredentials() []string { return nil }

func TestRegistryRegisterAndResolve(t *testing.T) {
	reg := NewRegistry()
	cmd := &mockCommand{name: "fs:list", namespace: "fs", desc: "List files"}

	if err := reg.Register(cmd); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	resolved, err := reg.Resolve("fs:list")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if resolved.Name() != "fs:list" {
		t.Errorf("expected fs:list, got %s", resolved.Name())
	}
}

func TestRegistryDuplicateRegister(t *testing.T) {
	reg := NewRegistry()
	cmd := &mockCommand{name: "fs:list", namespace: "fs"}

	if err := reg.Register(cmd); err != nil {
		t.Fatalf("first Register error: %v", err)
	}
	if err := reg.Register(cmd); err == nil {
		t.Error("expected error on duplicate register")
	}
}

func TestRegistryResolveNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestRegistryListByNamespace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockCommand{name: "fs:list", namespace: "fs"})
	reg.Register(&mockCommand{name: "fs:read", namespace: "fs"})
	reg.Register(&mockCommand{name: "github:pr:list", namespace: "github"})

	fsCmds := reg.List("fs")
	if len(fsCmds) != 2 {
		t.Errorf("expected 2 fs commands, got %d", len(fsCmds))
	}

	ghCmds := reg.List("github")
	if len(ghCmds) != 1 {
		t.Errorf("expected 1 github command, got %d", len(ghCmds))
	}

	allCmds := reg.List("")
	if len(allCmds) != 3 {
		t.Errorf("expected 3 total commands, got %d", len(allCmds))
	}
}

func TestRegistryDescribe(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockCommand{name: "fs:list", namespace: "fs"})

	schema, err := reg.Describe("fs:list")
	if err != nil {
		t.Fatalf("Describe error: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("expected type 'object', got %s", schema.Type)
	}
}

func TestRegistryDescribeNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Describe("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockCommand{name: "fs:list", namespace: "fs"})
	reg.Register(&mockCommand{name: "fs:read", namespace: "fs"})

	names := reg.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestRegistryNamespaces(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockCommand{name: "fs:list", namespace: "fs"})
	reg.Register(&mockCommand{name: "fs:read", namespace: "fs"})
	reg.Register(&mockCommand{name: "github:pr:list", namespace: "github"})

	ns := reg.Namespaces()
	if len(ns) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(ns))
	}
}

func TestRegistryMatchGlob(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockCommand{name: "fs:list", namespace: "fs"})
	reg.Register(&mockCommand{name: "fs:read", namespace: "fs"})
	reg.Register(&mockCommand{name: "fs:write", namespace: "fs"})
	reg.Register(&mockCommand{name: "github:pr:list", namespace: "github"})

	tests := []struct {
		pattern  string
		expected int
	}{
		{"fs:*", 3},
		{"github:*", 1},
		{"*", 4},
		{"fs:list", 1},
		{"http:*", 0},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			matches := reg.MatchGlob(tt.pattern)
			if len(matches) != tt.expected {
				t.Errorf("pattern %q: expected %d matches, got %d", tt.pattern, tt.expected, len(matches))
			}
		})
	}
}
