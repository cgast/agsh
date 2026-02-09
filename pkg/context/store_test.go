package context

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *BoltStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestBoltStoreSetGet(t *testing.T) {
	store := newTestStore(t)

	tests := []struct {
		name  string
		scope string
		key   string
		value any
	}{
		{"string value", ScopeSession, "name", "test-session"},
		{"number value", ScopeSession, "count", float64(42)},
		{"bool value", ScopeProject, "active", true},
		{"map value", ScopeProject, "config", map[string]any{"k": "v"}},
		{"slice value", ScopeSession, "items", []any{"a", "b", "c"}},
		{"nil value", ScopeStep, "empty", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.Set(tt.scope, tt.key, tt.value); err != nil {
				t.Fatalf("Set error: %v", err)
			}

			got, err := store.Get(tt.scope, tt.key)
			if err != nil {
				t.Fatalf("Get error: %v", err)
			}

			// JSON round-trip normalizes types, so compare via JSON
			if tt.value == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			switch expected := tt.value.(type) {
			case string:
				if got != expected {
					t.Errorf("expected %q, got %v", expected, got)
				}
			case float64:
				if got != expected {
					t.Errorf("expected %v, got %v", expected, got)
				}
			case bool:
				if got != expected {
					t.Errorf("expected %v, got %v", expected, got)
				}
			}
		})
	}
}

func TestBoltStoreGetNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Get(ScopeSession, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestBoltStoreGetInvalidScope(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Get("invalid_scope", "key")
	if err == nil {
		t.Error("expected error for invalid scope")
	}
}

func TestBoltStoreDelete(t *testing.T) {
	store := newTestStore(t)

	if err := store.Set(ScopeSession, "to_delete", "value"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	if err := store.Delete(ScopeSession, "to_delete"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	_, err := store.Get(ScopeSession, "to_delete")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestBoltStoreList(t *testing.T) {
	store := newTestStore(t)

	if err := store.Set(ScopeSession, "a", "alpha"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := store.Set(ScopeSession, "b", "beta"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := store.Set(ScopeSession, "c", "gamma"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	items, err := store.List(ScopeSession)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items["a"] != "alpha" {
		t.Errorf("expected a=alpha, got %v", items["a"])
	}
	if items["b"] != "beta" {
		t.Errorf("expected b=beta, got %v", items["b"])
	}
}

func TestBoltStoreListEmpty(t *testing.T) {
	store := newTestStore(t)

	items, err := store.List(ScopeProject)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestBoltStoreScopeIsolation(t *testing.T) {
	store := newTestStore(t)

	if err := store.Set(ScopeProject, "name", "project-val"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := store.Set(ScopeSession, "name", "session-val"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	projVal, err := store.Get(ScopeProject, "name")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	sessVal, err := store.Get(ScopeSession, "name")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if projVal != "project-val" {
		t.Errorf("project scope: expected project-val, got %v", projVal)
	}
	if sessVal != "session-val" {
		t.Errorf("session scope: expected session-val, got %v", sessVal)
	}
}

func TestBoltStoreOverwrite(t *testing.T) {
	store := newTestStore(t)

	if err := store.Set(ScopeSession, "key", "original"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := store.Set(ScopeSession, "key", "updated"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	got, err := store.Get(ScopeSession, "key")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got != "updated" {
		t.Errorf("expected 'updated', got %v", got)
	}
}

func TestBoltStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Write data.
	store1, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := store1.Set(ScopeProject, "goal", "test persistence"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	store1.Close()

	// Re-open and verify.
	store2, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	got, err := store2.Get(ScopeProject, "goal")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got != "test persistence" {
		t.Errorf("expected 'test persistence', got %v", got)
	}
}

func TestNewBoltStoreInvalidPath(t *testing.T) {
	_, err := NewBoltStore(filepath.Join(os.DevNull, "impossible", "path.db"))
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
