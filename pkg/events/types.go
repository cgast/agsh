package events

import "time"

// EventType identifies the kind of event emitted by the runtime.
type EventType string

const (
	EventCommandStart      EventType = "command.start"
	EventCommandEnd        EventType = "command.end"
	EventCommandError      EventType = "command.error"
	EventPipelineStart     EventType = "pipeline.start"
	EventPipelineEnd       EventType = "pipeline.end"
	EventPipelineStep      EventType = "pipeline.step"
	EventVerifyStart       EventType = "verify.start"
	EventVerifyResult      EventType = "verify.result"
	EventCheckpointSave    EventType = "checkpoint.save"
	EventCheckpointRestore EventType = "checkpoint.restore"
	EventContextChange     EventType = "context.change"
	EventPlanGenerated     EventType = "plan.generated"
	EventPlanApproval      EventType = "plan.approval_requested"
	EventPlanApproved      EventType = "plan.approved"
	EventPlanRejected      EventType = "plan.rejected"
	EventSpecLoaded        EventType = "spec.loaded"
	EventAgentMessage      EventType = "agent.message"
)

// Event represents a single runtime event.
type Event struct {
	Type      EventType     `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Data      any           `json:"data"`
	StepIndex int           `json:"step_index,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// NewEvent creates a new Event with the current timestamp.
func NewEvent(typ EventType, data any) Event {
	return Event{
		Type:      typ,
		Timestamp: time.Now(),
		Data:      data,
	}
}
