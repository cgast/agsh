package verify

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// CheckpointManager manages state snapshots for rollback.
type CheckpointManager interface {
	Save(name string, state SessionSnapshot) error
	Restore(name string) (SessionSnapshot, error)
	List() ([]CheckpointInfo, error)
	Diff(a, b string) ([]Change, error)
}

// SessionSnapshot captures the full state at a point in time.
type SessionSnapshot struct {
	ContextState map[string]map[string]any `json:"context_state"`
	WorkdirHash  string                    `json:"workdir_hash"`
	Timestamp    time.Time                 `json:"timestamp"`
}

// CheckpointInfo is metadata about a saved checkpoint.
type CheckpointInfo struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
}

// Change records a difference between two snapshots.
type Change struct {
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
	Type   string `json:"type"` // "added", "removed", "modified"
}

// FileCheckpointManager stores checkpoints as JSON files in a directory.
type FileCheckpointManager struct {
	dir string
}

// NewFileCheckpointManager creates a checkpoint manager that stores snapshots as files.
func NewFileCheckpointManager(dir string) (*FileCheckpointManager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}
	return &FileCheckpointManager{dir: dir}, nil
}

func (m *FileCheckpointManager) Save(name string, state SessionSnapshot) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	path := filepath.Join(m.dir, name+".json")
	return os.WriteFile(path, data, 0644)
}

func (m *FileCheckpointManager) Restore(name string) (SessionSnapshot, error) {
	path := filepath.Join(m.dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionSnapshot{}, fmt.Errorf("read checkpoint %q: %w", name, err)
	}
	var snap SessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return SessionSnapshot{}, fmt.Errorf("parse checkpoint %q: %w", name, err)
	}
	return snap, nil
}

func (m *FileCheckpointManager) List() ([]CheckpointInfo, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}

	var infos []CheckpointInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-5] // strip .json
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, CheckpointInfo{
			Name:      name,
			Timestamp: info.ModTime(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Timestamp.Before(infos[j].Timestamp)
	})
	return infos, nil
}

func (m *FileCheckpointManager) Diff(a, b string) ([]Change, error) {
	snapA, err := m.Restore(a)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint %q: %w", a, err)
	}
	snapB, err := m.Restore(b)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint %q: %w", b, err)
	}

	return diffSnapshots(snapA, snapB), nil
}

// diffSnapshots compares two snapshots and returns the changes.
func diffSnapshots(a, b SessionSnapshot) []Change {
	var changes []Change

	// Collect all scopes.
	scopes := make(map[string]bool)
	for s := range a.ContextState {
		scopes[s] = true
	}
	for s := range b.ContextState {
		scopes[s] = true
	}

	for scope := range scopes {
		aScope := a.ContextState[scope]
		bScope := b.ContextState[scope]

		// Keys in A.
		if aScope != nil {
			for key, valA := range aScope {
				if bScope == nil {
					changes = append(changes, Change{Scope: scope, Key: key, Before: valA, Type: "removed"})
				} else if valB, ok := bScope[key]; !ok {
					changes = append(changes, Change{Scope: scope, Key: key, Before: valA, Type: "removed"})
				} else if fmt.Sprintf("%v", valA) != fmt.Sprintf("%v", valB) {
					changes = append(changes, Change{Scope: scope, Key: key, Before: valA, After: valB, Type: "modified"})
				}
			}
		}

		// Keys only in B.
		if bScope != nil {
			for key, valB := range bScope {
				if aScope == nil || aScope[key] == nil {
					if aScope != nil {
						if _, exists := aScope[key]; exists {
							continue
						}
					}
					changes = append(changes, Change{Scope: scope, Key: key, After: valB, Type: "added"})
				}
			}
		}
	}

	return changes
}

// CaptureSnapshot takes a snapshot of the current context store state.
func CaptureSnapshot(store agshctx.ContextStore, workdir string) (SessionSnapshot, error) {
	scopes := []string{
		agshctx.ScopeProject,
		agshctx.ScopeSession,
		agshctx.ScopeStep,
	}

	state := make(map[string]map[string]any)
	for _, scope := range scopes {
		items, err := store.List(scope)
		if err != nil {
			return SessionSnapshot{}, fmt.Errorf("list scope %s: %w", scope, err)
		}
		if len(items) > 0 {
			state[scope] = items
		}
	}

	hash := ""
	if workdir != "" {
		h, err := hashDir(workdir)
		if err == nil {
			hash = h
		}
	}

	return SessionSnapshot{
		ContextState: state,
		WorkdirHash:  hash,
		Timestamp:    time.Now(),
	}, nil
}

// RestoreSnapshot writes a snapshot back into the context store.
func RestoreSnapshot(store agshctx.ContextStore, snap SessionSnapshot) error {
	for scope, items := range snap.ContextState {
		for key, val := range items {
			if err := store.Set(scope, key, val); err != nil {
				return fmt.Errorf("restore %s/%s: %w", scope, key, err)
			}
		}
	}
	return nil
}

// hashDir computes a quick hash of a directory's file listing for change detection.
func hashDir(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(h, "%s:%d:%d\n", rel, info.Size(), info.ModTime().Unix())
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
