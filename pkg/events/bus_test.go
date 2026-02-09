package events

import (
	"testing"
	"time"
)

func TestMemoryBusPublishSubscribe(t *testing.T) {
	bus := NewMemoryBus()
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	bus.Publish(NewEvent(EventCommandStart, "test"))

	select {
	case event := <-ch:
		if event.Type != EventCommandStart {
			t.Errorf("expected EventCommandStart, got %s", event.Type)
		}
		if event.Data != "test" {
			t.Errorf("expected data 'test', got %v", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestMemoryBusFilter(t *testing.T) {
	bus := NewMemoryBus()
	ch := bus.Subscribe(EventCommandEnd)
	defer bus.Unsubscribe(ch)

	bus.Publish(NewEvent(EventCommandStart, "should-be-filtered"))
	bus.Publish(NewEvent(EventCommandEnd, "should-arrive"))

	select {
	case event := <-ch:
		if event.Type != EventCommandEnd {
			t.Errorf("expected EventCommandEnd, got %s", event.Type)
		}
		if event.Data != "should-arrive" {
			t.Errorf("expected data 'should-arrive', got %v", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}

	// Ensure the filtered event didn't arrive.
	select {
	case event := <-ch:
		t.Errorf("unexpected event: %v", event)
	case <-time.After(50 * time.Millisecond):
		// Good — no event arrived.
	}
}

func TestMemoryBusMultipleSubscribers(t *testing.T) {
	bus := NewMemoryBus()
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Publish(NewEvent(EventPipelineStart, "pipeline-1"))

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			if event.Type != EventPipelineStart {
				t.Errorf("expected EventPipelineStart, got %s", event.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestMemoryBusHistory(t *testing.T) {
	bus := NewMemoryBus()

	t1 := time.Now()
	bus.Publish(NewEvent(EventCommandStart, "first"))
	time.Sleep(10 * time.Millisecond)
	t2 := time.Now()
	bus.Publish(NewEvent(EventCommandEnd, "second"))

	all := bus.History(t1)
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}

	since := bus.History(t2)
	if len(since) != 1 {
		t.Fatalf("expected 1 event since t2, got %d", len(since))
	}
	if since[0].Data != "second" {
		t.Errorf("expected 'second', got %v", since[0].Data)
	}
}

func TestMemoryBusHistoryEmpty(t *testing.T) {
	bus := NewMemoryBus()
	events := bus.History(time.Time{})
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestMemoryBusUnsubscribe(t *testing.T) {
	bus := NewMemoryBus()
	ch := bus.Subscribe()
	bus.Unsubscribe(ch)

	// Channel should be closed after unsubscribe.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestNewEvent(t *testing.T) {
	event := NewEvent(EventCommandStart, map[string]string{"cmd": "fs:list"})

	if event.Type != EventCommandStart {
		t.Errorf("expected EventCommandStart, got %s", event.Type)
	}
	if event.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
}
