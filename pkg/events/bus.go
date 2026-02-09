package events

import (
	"sync"
	"time"
)

// EventBus provides publish/subscribe for runtime events.
type EventBus interface {
	Publish(event Event)
	Subscribe(filter ...EventType) <-chan Event
	Unsubscribe(ch <-chan Event)
	History(since time.Time) []Event
}

type subscriber struct {
	ch     chan Event
	filter map[EventType]bool // empty means all events
}

// MemoryBus is an in-memory implementation of EventBus.
type MemoryBus struct {
	mu          sync.RWMutex
	subscribers []subscriber
	history     []Event
}

// NewMemoryBus creates a new in-memory event bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		history: make([]Event, 0, 256),
	}
}

func (b *MemoryBus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.Lock()
	b.history = append(b.history, event)
	subs := make([]subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	for _, sub := range subs {
		if len(sub.filter) > 0 && !sub.filter[event.Type] {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// Drop event if subscriber is slow; avoids blocking the publisher.
		}
	}
}

func (b *MemoryBus) Subscribe(filter ...EventType) <-chan Event {
	ch := make(chan Event, 64)
	sub := subscriber{ch: ch}
	if len(filter) > 0 {
		sub.filter = make(map[EventType]bool, len(filter))
		for _, f := range filter {
			sub.filter[f] = true
		}
	}

	b.mu.Lock()
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()

	return ch
}

func (b *MemoryBus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subscribers {
		if sub.ch == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(sub.ch)
			return
		}
	}
}

func (b *MemoryBus) History(since time.Time) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []Event
	for _, e := range b.history {
		if !e.Timestamp.Before(since) {
			result = append(result, e)
		}
	}
	return result
}
