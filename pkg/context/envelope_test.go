package context

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEnvelope(t *testing.T) {
	env := NewEnvelope("hello", "text/plain", "test")

	if env.Payload != "hello" {
		t.Errorf("expected payload 'hello', got %v", env.Payload)
	}
	if env.Meta.ContentType != "text/plain" {
		t.Errorf("expected content type 'text/plain', got %s", env.Meta.ContentType)
	}
	if env.Meta.Source != "test" {
		t.Errorf("expected source 'test', got %s", env.Meta.Source)
	}
	if env.Meta.Tags == nil {
		t.Error("expected tags map to be initialized")
	}
	if len(env.Provenance) != 0 {
		t.Errorf("expected empty provenance, got %d steps", len(env.Provenance))
	}
	if env.Meta.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestEnvelopeAddStep(t *testing.T) {
	env := NewEnvelope("data", "text/plain", "test")
	step := Step{
		Command:   "fs:list",
		Args:      []string{"./src"},
		Timestamp: time.Now(),
		Duration:  100 * time.Millisecond,
		Status:    "ok",
	}

	env.AddStep(step)

	if len(env.Provenance) != 1 {
		t.Fatalf("expected 1 step, got %d", len(env.Provenance))
	}
	if env.Provenance[0].Command != "fs:list" {
		t.Errorf("expected command 'fs:list', got %s", env.Provenance[0].Command)
	}
	if env.Provenance[0].Status != "ok" {
		t.Errorf("expected status 'ok', got %s", env.Provenance[0].Status)
	}
}

func TestEnvelopeJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		payload     any
		contentType string
		source      string
	}{
		{
			name:        "string payload",
			payload:     "hello world",
			contentType: "text/plain",
			source:      "test",
		},
		{
			name:        "map payload",
			payload:     map[string]any{"key": "value", "count": float64(42)},
			contentType: "application/json",
			source:      "fs:read",
		},
		{
			name:        "slice payload",
			payload:     []any{"a", "b", "c"},
			contentType: "application/json",
			source:      "fs:list",
		},
		{
			name:        "nil payload",
			payload:     nil,
			contentType: "text/plain",
			source:      "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewEnvelope(tt.payload, tt.contentType, tt.source)
			original.Meta.Tags["env"] = "test"
			original.AddStep(Step{
				Command:   "test:cmd",
				Args:      []string{"--flag"},
				Timestamp: time.Date(2025, 2, 9, 12, 0, 0, 0, time.UTC),
				Duration:  50 * time.Millisecond,
				Status:    "ok",
			})

			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded Envelope
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if decoded.Meta.ContentType != tt.contentType {
				t.Errorf("content type: expected %s, got %s", tt.contentType, decoded.Meta.ContentType)
			}
			if decoded.Meta.Source != tt.source {
				t.Errorf("source: expected %s, got %s", tt.source, decoded.Meta.Source)
			}
			if decoded.Meta.Tags["env"] != "test" {
				t.Errorf("tags: expected env=test, got %v", decoded.Meta.Tags)
			}
			if len(decoded.Provenance) != 1 {
				t.Fatalf("provenance: expected 1 step, got %d", len(decoded.Provenance))
			}
			if decoded.Provenance[0].Command != "test:cmd" {
				t.Errorf("step command: expected test:cmd, got %s", decoded.Provenance[0].Command)
			}
		})
	}
}

func TestStepJSONRoundTrip(t *testing.T) {
	original := Step{
		Command:   "github:pr:list",
		Args:      []string{"--state=open", "--repo=cgast/agsh"},
		Timestamp: time.Date(2025, 2, 9, 14, 30, 0, 0, time.UTC),
		Duration:  2500 * time.Millisecond,
		Status:    "ok",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Step
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Command != original.Command {
		t.Errorf("command: expected %s, got %s", original.Command, decoded.Command)
	}
	if len(decoded.Args) != 2 {
		t.Fatalf("args: expected 2, got %d", len(decoded.Args))
	}
	if decoded.Duration != original.Duration {
		t.Errorf("duration: expected %v, got %v", original.Duration, decoded.Duration)
	}
	if decoded.Status != "ok" {
		t.Errorf("status: expected ok, got %s", decoded.Status)
	}
}

func TestPayloadString(t *testing.T) {
	tests := []struct {
		name     string
		payload  any
		expected string
	}{
		{"string", "hello", "hello"},
		{"bytes", []byte("world"), "world"},
		{"number", float64(42), "42"},
		{"map", map[string]string{"k": "v"}, `{"k":"v"}`},
		{"nil", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := NewEnvelope(tt.payload, "text/plain", "test")
			got := env.PayloadString()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestMetadataJSONRoundTrip(t *testing.T) {
	original := Metadata{
		ContentType: "application/json",
		Tags:        map[string]string{"version": "1.0", "env": "prod"},
		CreatedAt:   time.Date(2025, 2, 9, 12, 0, 0, 0, time.UTC),
		Source:      "github:repo:info",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Metadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ContentType != original.ContentType {
		t.Errorf("content type: expected %s, got %s", original.ContentType, decoded.ContentType)
	}
	if decoded.Tags["version"] != "1.0" {
		t.Errorf("tags: expected version=1.0, got %v", decoded.Tags)
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("created at: expected %v, got %v", original.CreatedAt, decoded.CreatedAt)
	}
}
