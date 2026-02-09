package context

import (
	"encoding/json"
	"time"
)

// Envelope is the core data type flowing through all agsh pipelines.
// Every value passed between commands is wrapped in an Envelope carrying
// the payload alongside metadata and provenance information.
type Envelope struct {
	Payload    any        `json:"payload"`
	Meta       Metadata   `json:"meta"`
	Provenance []Step     `json:"provenance"`
}

// Metadata carries information about the envelope's content and origin.
type Metadata struct {
	ContentType string            `json:"content_type"`
	Tags        map[string]string `json:"tags"`
	CreatedAt   time.Time         `json:"created_at"`
	Source      string            `json:"source"`
}

// Step records a single operation in the provenance chain.
type Step struct {
	Command   string        `json:"command"`
	Args      []string      `json:"args"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	Status    string        `json:"status"` // "ok", "error", "skipped"
}

// NewEnvelope creates a new Envelope with the given payload, content type, and source.
func NewEnvelope(payload any, contentType, source string) Envelope {
	return Envelope{
		Payload: payload,
		Meta: Metadata{
			ContentType: contentType,
			Tags:        make(map[string]string),
			CreatedAt:   time.Now(),
			Source:      source,
		},
		Provenance: []Step{},
	}
}

// AddStep appends a provenance step to the envelope.
func (e *Envelope) AddStep(step Step) {
	e.Provenance = append(e.Provenance, step)
}

// MarshalJSON implements custom JSON marshaling for Envelope.
// time.Duration is serialized as nanoseconds by default in JSON;
// we keep that behavior for machine readability.
func (e Envelope) MarshalJSON() ([]byte, error) {
	type Alias Envelope
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(e),
	})
}

// PayloadString returns the payload as a string if possible.
// Returns the JSON representation for non-string payloads.
func (e *Envelope) PayloadString() string {
	switch v := e.Payload.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
