package context

import (
	gocontext "context"
	"fmt"
	"time"
)

// CommandExecutor is the interface that pipeline uses to execute commands.
// This avoids a direct dependency on pkg/platform.
type CommandExecutor interface {
	Execute(ctx gocontext.Context, name string, input Envelope, store ContextStore) (Envelope, error)
}

// EventPublisher is the interface for emitting events during pipeline execution.
// This avoids a direct dependency on pkg/events.
type EventPublisher interface {
	PublishPipelineEvent(eventType string, data any, stepIndex int, duration time.Duration)
}

// Pipeline defines a sequence of commands to execute.
type Pipeline struct {
	Steps    []PipelineStep
	Context  ContextStore
	Executor CommandExecutor
	Events   EventPublisher
}

// PipelineStep defines a single step within a pipeline.
type PipelineStep struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Intent  string   `json:"intent"`
	OnError string   `json:"on_error"` // "stop", "skip", "retry"
}

// PipelineResult holds the outcome of a pipeline execution.
type PipelineResult struct {
	Steps   []StepResult `json:"steps"`
	Success bool         `json:"success"`
	Output  Envelope     `json:"output"`
}

// StepResult records the outcome of a single pipeline step.
type StepResult struct {
	Step     PipelineStep  `json:"step"`
	Output   Envelope      `json:"output"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Status   string        `json:"status"` // "ok", "error", "skipped"
}

// Run executes the pipeline, passing envelopes between steps.
func (p *Pipeline) Run(ctx gocontext.Context, input Envelope) (PipelineResult, error) {
	if p.Executor == nil {
		return PipelineResult{}, fmt.Errorf("pipeline: no executor configured")
	}

	result := PipelineResult{
		Steps:   make([]StepResult, 0, len(p.Steps)),
		Success: true,
	}

	current := input

	p.publishEvent("pipeline.start", map[string]any{
		"step_count": len(p.Steps),
	}, 0, 0)

	for i, step := range p.Steps {
		// Set step context if store is available.
		if p.Context != nil {
			p.Context.Set(ScopeStep, "command", step.Command)
			p.Context.Set(ScopeStep, "index", i)
			if step.Intent != "" {
				p.Context.Set(ScopeStep, "intent", step.Intent)
			}
		}

		p.publishEvent("command.start", map[string]any{
			"command": step.Command,
			"args":    step.Args,
			"intent":  step.Intent,
		}, i, 0)

		start := time.Now()
		output, err := p.Executor.Execute(ctx, step.Command, current, p.Context)
		duration := time.Since(start)

		sr := StepResult{
			Step:     step,
			Duration: duration,
		}

		if err != nil {
			sr.Status = "error"
			sr.Error = err.Error()
			result.Steps = append(result.Steps, sr)

			p.publishEvent("command.error", map[string]any{
				"command": step.Command,
				"error":   err.Error(),
			}, i, duration)

			onError := step.OnError
			if onError == "" {
				onError = "stop"
			}

			switch onError {
			case "skip":
				sr.Status = "skipped"
				continue
			case "stop":
				result.Success = false
				p.publishEvent("pipeline.end", map[string]any{
					"success": false,
					"error":   err.Error(),
					"step":    i,
				}, i, 0)
				return result, fmt.Errorf("pipeline stopped at step %d (%s): %w", i, step.Command, err)
			default:
				result.Success = false
				p.publishEvent("pipeline.end", map[string]any{
					"success": false,
					"error":   err.Error(),
					"step":    i,
				}, i, 0)
				return result, fmt.Errorf("pipeline stopped at step %d (%s): %w", i, step.Command, err)
			}
		}

		// Record provenance.
		output.AddStep(Step{
			Command:   step.Command,
			Args:      step.Args,
			Timestamp: start,
			Duration:  duration,
			Status:    "ok",
		})

		sr.Status = "ok"
		sr.Output = output
		result.Steps = append(result.Steps, sr)

		p.publishEvent("command.end", map[string]any{
			"command": step.Command,
			"status":  "ok",
		}, i, duration)

		// Pass output as input to the next step.
		current = output
	}

	result.Output = current

	p.publishEvent("pipeline.end", map[string]any{
		"success":    true,
		"step_count": len(p.Steps),
	}, len(p.Steps)-1, 0)

	return result, nil
}

func (p *Pipeline) publishEvent(eventType string, data any, stepIndex int, duration time.Duration) {
	if p.Events != nil {
		p.Events.PublishPipelineEvent(eventType, data, stepIndex, duration)
	}
}
