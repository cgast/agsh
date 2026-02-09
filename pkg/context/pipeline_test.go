package context

import (
	gocontext "context"
	"fmt"
	"testing"
	"time"
)

// testExecutor is a mock command executor for testing.
type testExecutor struct {
	commands map[string]func(gocontext.Context, Envelope, ContextStore) (Envelope, error)
}

func newTestExecutor() *testExecutor {
	return &testExecutor{
		commands: make(map[string]func(gocontext.Context, Envelope, ContextStore) (Envelope, error)),
	}
}

func (e *testExecutor) Register(name string, fn func(gocontext.Context, Envelope, ContextStore) (Envelope, error)) {
	e.commands[name] = fn
}

func (e *testExecutor) Execute(ctx gocontext.Context, name string, input Envelope, store ContextStore) (Envelope, error) {
	fn, ok := e.commands[name]
	if !ok {
		return Envelope{}, fmt.Errorf("command not found: %s", name)
	}
	return fn(ctx, input, store)
}

// testEventPublisher records pipeline events for verification.
type testEventPublisher struct {
	events []struct {
		Type      string
		Data      any
		StepIndex int
		Duration  time.Duration
	}
}

func (p *testEventPublisher) PublishPipelineEvent(eventType string, data any, stepIndex int, duration time.Duration) {
	p.events = append(p.events, struct {
		Type      string
		Data      any
		StepIndex int
		Duration  time.Duration
	}{eventType, data, stepIndex, duration})
}

func TestPipelineBasicExecution(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("step1", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("output1", "text/plain", "step1"), nil
	})
	exec.Register("step2", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		// Receives output from step1.
		prev := input.PayloadString()
		return NewEnvelope(prev+"+output2", "text/plain", "step2"), nil
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "step1"},
			{Command: "step2"},
		},
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !result.Success {
		t.Error("expected success")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if result.Output.PayloadString() != "output1+output2" {
		t.Errorf("expected 'output1+output2', got %q", result.Output.PayloadString())
	}
}

func TestPipelineEnvelopeFlows(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("upper", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		s := input.PayloadString()
		return NewEnvelope("UPPER:"+s, "text/plain", "upper"), nil
	})
	exec.Register("wrap", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		s := input.PayloadString()
		return NewEnvelope("["+s+"]", "text/plain", "wrap"), nil
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "upper"},
			{Command: "wrap"},
		},
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope("hello", "text/plain", "input"))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	expected := "[UPPER:hello]"
	if result.Output.PayloadString() != expected {
		t.Errorf("expected %q, got %q", expected, result.Output.PayloadString())
	}
}

func TestPipelineProvenance(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("cmd1", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("data", "text/plain", "cmd1"), nil
	})
	exec.Register("cmd2", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		// Pass through input provenance.
		env := NewEnvelope(input.Payload, "text/plain", "cmd2")
		env.Provenance = input.Provenance
		return env, nil
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "cmd1"},
			{Command: "cmd2"},
		},
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Each step adds provenance; cmd2 preserved cmd1's.
	if len(result.Output.Provenance) != 2 {
		t.Fatalf("expected 2 provenance steps, got %d", len(result.Output.Provenance))
	}
	if result.Output.Provenance[0].Command != "cmd1" {
		t.Errorf("expected first provenance cmd1, got %s", result.Output.Provenance[0].Command)
	}
	if result.Output.Provenance[1].Command != "cmd2" {
		t.Errorf("expected second provenance cmd2, got %s", result.Output.Provenance[1].Command)
	}
}

func TestPipelineErrorStops(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("fail", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return Envelope{}, fmt.Errorf("intentional error")
	})
	exec.Register("after", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("should not reach", "text/plain", "after"), nil
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "fail"},
			{Command: "after"},
		},
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err == nil {
		t.Fatal("expected error")
	}

	if result.Success {
		t.Error("expected failure")
	}
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "error" {
		t.Errorf("expected status 'error', got %s", result.Steps[0].Status)
	}
}

func TestPipelineErrorSkip(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("fail", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return Envelope{}, fmt.Errorf("skip this error")
	})
	exec.Register("after", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("reached", "text/plain", "after"), nil
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "fail", OnError: "skip"},
			{Command: "after"},
		},
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope("initial", "text/plain", "test"))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !result.Success {
		t.Error("expected success with skip")
	}
	if result.Output.PayloadString() != "reached" {
		t.Errorf("expected 'reached', got %q", result.Output.PayloadString())
	}
}

func TestPipelineEvents(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("cmd", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("data", "text/plain", "cmd"), nil
	})

	pub := &testEventPublisher{}

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "cmd"},
		},
		Executor: exec,
		Events:   pub,
	}

	_, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Should have: pipeline.start, command.start, command.end, pipeline.end
	if len(pub.events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(pub.events))
	}

	expectedTypes := []string{"pipeline.start", "command.start", "command.end", "pipeline.end"}
	for i, expected := range expectedTypes {
		if pub.events[i].Type != expected {
			t.Errorf("event %d: expected %s, got %s", i, expected, pub.events[i].Type)
		}
	}
}

func TestPipelineWithContextStore(t *testing.T) {
	store := newTestStore(t)

	exec := newTestExecutor()
	exec.Register("writer", func(_ gocontext.Context, _ Envelope, s ContextStore) (Envelope, error) {
		if s != nil {
			s.Set(ScopeSession, "written_by", "writer_cmd")
		}
		return NewEnvelope("wrote data", "text/plain", "writer"), nil
	})
	exec.Register("reader", func(_ gocontext.Context, _ Envelope, s ContextStore) (Envelope, error) {
		if s != nil {
			val, err := s.Get(ScopeSession, "written_by")
			if err != nil {
				return Envelope{}, err
			}
			return NewEnvelope(val, "text/plain", "reader"), nil
		}
		return Envelope{}, fmt.Errorf("no store")
	})

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "writer"},
			{Command: "reader"},
		},
		Context:  store,
		Executor: exec,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if result.Output.PayloadString() != "writer_cmd" {
		t.Errorf("expected 'writer_cmd', got %q", result.Output.PayloadString())
	}
}

func TestPipelineNoExecutor(t *testing.T) {
	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "cmd"},
		},
	}

	_, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err == nil {
		t.Error("expected error with nil executor")
	}
}

// testVerifier is a mock step verifier for testing.
type testVerifier struct {
	results map[int]bool // step index → pass/fail
}

func (v *testVerifier) VerifyStep(stepIndex int, envelope Envelope) (bool, string, error) {
	passed, ok := v.results[stepIndex]
	if !ok {
		return true, "no assertion", nil
	}
	if passed {
		return true, "verification passed", nil
	}
	return false, "verification failed", nil
}

// testCheckpointer is a mock checkpointer for testing.
type testCheckpointer struct {
	saved    []string
	restored []string
}

func (c *testCheckpointer) SaveCheckpoint(name string) error {
	c.saved = append(c.saved, name)
	return nil
}

func (c *testCheckpointer) RestoreCheckpoint(name string) error {
	c.restored = append(c.restored, name)
	return nil
}

func TestPipelineVerification(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("step1", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("output1", "text/plain", "step1"), nil
	})
	exec.Register("step2", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("output2", "text/plain", "step2"), nil
	})

	verifier := &testVerifier{results: map[int]bool{0: true, 1: true}}

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "step1", Intent: "produce output1"},
			{Command: "step2", Intent: "produce output2"},
		},
		Executor: exec,
		Verifier: verifier,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	for _, sr := range result.Steps {
		if sr.VerifyPassed == nil {
			t.Error("expected VerifyPassed to be set")
		} else if !*sr.VerifyPassed {
			t.Error("expected verification to pass")
		}
	}
}

func TestPipelineVerificationFails(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("step1", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("bad output", "text/plain", "step1"), nil
	})
	exec.Register("step2", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("should not reach", "text/plain", "step2"), nil
	})

	verifier := &testVerifier{results: map[int]bool{0: false}}

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "step1", Intent: "produce good output"},
			{Command: "step2"},
		},
		Executor: exec,
		Verifier: verifier,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err == nil {
		t.Fatal("expected error from verification failure")
	}
	if result.Success {
		t.Error("expected failure")
	}
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "verify_failed" {
		t.Errorf("expected status 'verify_failed', got %s", result.Steps[0].Status)
	}
}

func TestPipelineVerificationSkip(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("step1", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("bad", "text/plain", "step1"), nil
	})
	exec.Register("step2", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("good", "text/plain", "step2"), nil
	})

	verifier := &testVerifier{results: map[int]bool{0: false, 1: true}}

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "step1", OnError: "skip"},
			{Command: "step2"},
		},
		Executor: exec,
		Verifier: verifier,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Success {
		t.Error("expected success with skip on verify failure")
	}
}

func TestPipelineCheckpointing(t *testing.T) {
	exec := newTestExecutor()
	exec.Register("read-cmd", func(_ gocontext.Context, _ Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("data", "text/plain", "read"), nil
	})
	exec.Register("write-cmd", func(_ gocontext.Context, input Envelope, _ ContextStore) (Envelope, error) {
		return NewEnvelope("written", "text/plain", "write"), nil
	})

	cp := &testCheckpointer{}

	p := &Pipeline{
		Steps: []PipelineStep{
			{Command: "read-cmd"},
			{Command: "write-cmd", CheckpointBefore: true},
		},
		Executor:     exec,
		Checkpointer: cp,
	}

	result, err := p.Run(gocontext.Background(), NewEnvelope(nil, "", ""))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(cp.saved) != 1 {
		t.Errorf("expected 1 checkpoint saved, got %d", len(cp.saved))
	}
	if cp.saved[0] != "step-1-write-cmd" {
		t.Errorf("checkpoint name = %q, want %q", cp.saved[0], "step-1-write-cmd")
	}
	// The step with checkpoint should record the checkpoint name.
	if result.Steps[1].CheckpointSaved != "step-1-write-cmd" {
		t.Errorf("CheckpointSaved = %q", result.Steps[1].CheckpointSaved)
	}
}

func TestPipelineEmptySteps(t *testing.T) {
	exec := newTestExecutor()

	p := &Pipeline{
		Steps:    []PipelineStep{},
		Executor: exec,
	}

	input := NewEnvelope("passthrough", "text/plain", "test")
	result, err := p.Run(gocontext.Background(), input)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !result.Success {
		t.Error("expected success for empty pipeline")
	}
	if result.Output.PayloadString() != "passthrough" {
		t.Errorf("expected input passthrough, got %q", result.Output.PayloadString())
	}
}
