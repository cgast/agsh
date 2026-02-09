package platform

import (
	"fmt"
	"strings"
	"sync"
)

// Registry holds all registered platform commands, keyed by full name.
type Registry struct {
	mu       sync.RWMutex
	commands map[string]PlatformCommand
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]PlatformCommand),
	}
}

// Register adds a command to the registry. Returns an error if a command
// with the same name is already registered.
func (r *Registry) Register(cmd PlatformCommand) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := cmd.Name()
	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("command already registered: %s", name)
	}
	r.commands[name] = cmd
	return nil
}

// Resolve looks up a command by its full name (e.g. "fs:list").
func (r *Registry) Resolve(name string) (PlatformCommand, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	if !ok {
		return nil, fmt.Errorf("command not found: %s", name)
	}
	return cmd, nil
}

// List returns all commands in a given namespace. If namespace is empty,
// returns all commands.
func (r *Registry) List(namespace string) []PlatformCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []PlatformCommand
	for _, cmd := range r.commands {
		if namespace == "" || cmd.Namespace() == namespace {
			result = append(result, cmd)
		}
	}
	return result
}

// Describe returns the input schema for a command.
func (r *Registry) Describe(name string) (Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	if !ok {
		return Schema{}, fmt.Errorf("command not found: %s", name)
	}
	return cmd.InputSchema(), nil
}

// Names returns all registered command names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	return names
}

// Namespaces returns all unique namespaces.
func (r *Registry) Namespaces() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	for _, cmd := range r.commands {
		ns := cmd.Namespace()
		if ns != "" && !seen[ns] {
			seen[ns] = true
		}
	}

	result := make([]string, 0, len(seen))
	for ns := range seen {
		result = append(result, ns)
	}
	return result
}

// MatchGlob returns all commands matching a glob pattern like "fs:*" or "github:pr:*".
func (r *Registry) MatchGlob(pattern string) []PlatformCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []PlatformCommand
	for name, cmd := range r.commands {
		if matchGlob(pattern, name) {
			result = append(result, cmd)
		}
	}
	return result
}

// matchGlob checks if name matches a simple glob pattern (only trailing * supported).
func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	return pattern == name
}
