package context

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Predefined scopes for the context store.
const (
	ScopeProject = "project"  // goals, constraints, guidelines (loaded from config)
	ScopeSession = "session"  // current session state, working memory
	ScopeStep    = "step"     // current pipeline step context (ephemeral)
	ScopeHistory = "history"  // append-only log of all operations
)

// ContextStore provides scoped key-value storage for pipeline state.
type ContextStore interface {
	Get(scope, key string) (any, error)
	Set(scope, key string, value any) error
	Delete(scope, key string) error
	List(scope string) (map[string]any, error)
	Close() error
}

// BoltStore is a bbolt-backed implementation of ContextStore.
type BoltStore struct {
	db *bolt.DB
	mu sync.RWMutex
}

// NewBoltStore creates a new bbolt-backed context store at the given path.
func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}

	// Pre-create scope buckets.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, scope := range []string{ScopeProject, ScopeSession, ScopeStep, ScopeHistory} {
			if _, err := tx.CreateBucketIfNotExists([]byte(scope)); err != nil {
				return fmt.Errorf("create bucket %s: %w", scope, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	return &BoltStore{db: db}, nil
}

func (s *BoltStore) Get(scope, key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result any
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(scope))
		if b == nil {
			return fmt.Errorf("scope not found: %s", scope)
		}
		data := b.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("key not found: %s/%s", scope, key)
		}
		return json.Unmarshal(data, &result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *BoltStore) Set(scope, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(scope))
		if b == nil {
			return fmt.Errorf("scope not found: %s", scope)
		}
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("marshal value: %w", err)
		}
		return b.Put([]byte(key), data)
	})
}

func (s *BoltStore) Delete(scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(scope))
		if b == nil {
			return fmt.Errorf("scope not found: %s", scope)
		}
		return b.Delete([]byte(key))
	})
}

func (s *BoltStore) List(scope string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]any)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(scope))
		if b == nil {
			return fmt.Errorf("scope not found: %s", scope)
		}
		return b.ForEach(func(k, v []byte) error {
			var val any
			if err := json.Unmarshal(v, &val); err != nil {
				return fmt.Errorf("unmarshal key %s: %w", string(k), err)
			}
			result[string(k)] = val
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}
