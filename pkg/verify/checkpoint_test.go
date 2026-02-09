package verify

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestFileCheckpointSaveRestore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	mgr, err := NewFileCheckpointManager(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointManager: %v", err)
	}

	snap := SessionSnapshot{
		ContextState: map[string]map[string]any{
			"session": {"key1": "val1", "key2": float64(42)},
			"project": {"name": "test"},
		},
		WorkdirHash: "abc123",
		Timestamp:   time.Now(),
	}

	if err := mgr.Save("checkpoint-1", snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	restored, err := mgr.Restore("checkpoint-1")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if restored.WorkdirHash != "abc123" {
		t.Errorf("WorkdirHash = %q, want %q", restored.WorkdirHash, "abc123")
	}
	if restored.ContextState["session"]["key1"] != "val1" {
		t.Errorf("session.key1 = %v", restored.ContextState["session"]["key1"])
	}
	if restored.ContextState["project"]["name"] != "test" {
		t.Errorf("project.name = %v", restored.ContextState["project"]["name"])
	}
}

func TestFileCheckpointRestoreMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	mgr, err := NewFileCheckpointManager(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointManager: %v", err)
	}

	_, err = mgr.Restore("nonexistent")
	if err == nil {
		t.Error("expected error for missing checkpoint")
	}
}

func TestFileCheckpointList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	mgr, err := NewFileCheckpointManager(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointManager: %v", err)
	}

	snap := SessionSnapshot{Timestamp: time.Now()}
	mgr.Save("cp-a", snap)
	mgr.Save("cp-b", snap)
	mgr.Save("cp-c", snap)

	infos, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 3 {
		t.Errorf("List() len = %d, want 3", len(infos))
	}
}

func TestFileCheckpointDiff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	mgr, err := NewFileCheckpointManager(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointManager: %v", err)
	}

	snapA := SessionSnapshot{
		ContextState: map[string]map[string]any{
			"session": {"key1": "val1", "key2": "val2"},
		},
		Timestamp: time.Now(),
	}
	snapB := SessionSnapshot{
		ContextState: map[string]map[string]any{
			"session": {"key1": "val1-changed", "key3": "new"},
		},
		Timestamp: time.Now(),
	}

	mgr.Save("a", snapA)
	mgr.Save("b", snapB)

	changes, err := mgr.Diff("a", "b")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(changes) == 0 {
		t.Error("expected changes")
	}

	// Should have: key1 modified, key2 removed, key3 added.
	types := make(map[string]int)
	for _, c := range changes {
		types[c.Type]++
	}
	if types["modified"] != 1 {
		t.Errorf("modified changes = %d, want 1", types["modified"])
	}
	if types["removed"] != 1 {
		t.Errorf("removed changes = %d, want 1", types["removed"])
	}
	if types["added"] != 1 {
		t.Errorf("added changes = %d, want 1", types["added"])
	}
}

func TestCaptureSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()

	store.Set(agshctx.ScopeSession, "name", "test")
	store.Set(agshctx.ScopeSession, "count", 42)

	snap, err := CaptureSnapshot(store, "")
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}

	if snap.ContextState["session"]["name"] != "test" {
		t.Errorf("session.name = %v", snap.ContextState["session"]["name"])
	}
}

func TestCaptureSnapshotWithWorkdir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()

	workdir := filepath.Join(t.TempDir(), "work")
	os.MkdirAll(workdir, 0755)
	os.WriteFile(filepath.Join(workdir, "test.txt"), []byte("hello"), 0644)

	snap, err := CaptureSnapshot(store, workdir)
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}

	if snap.WorkdirHash == "" {
		t.Error("WorkdirHash should not be empty with files")
	}
}

func TestRestoreSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()

	snap := SessionSnapshot{
		ContextState: map[string]map[string]any{
			"session": {"restored_key": "restored_val"},
		},
	}

	if err := RestoreSnapshot(store, snap); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	val, err := store.Get(agshctx.ScopeSession, "restored_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "restored_val" {
		t.Errorf("restored value = %v, want %q", val, "restored_val")
	}
}
