package verify

import (
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestEngineAllPass(t *testing.T) {
	env := agshctx.NewEnvelope("hello world", "text/plain", "test")
	intent := Intent{
		Description: "output should be non-empty and contain hello",
		Assertions: []Assertion{
			{Type: "not_empty", Target: "output"},
			{Type: "contains", Target: "output", Expected: "hello"},
		},
	}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed {
		t.Error("expected all assertions to pass")
	}
	if len(result.Results) != 2 {
		t.Errorf("results count = %d, want 2", len(result.Results))
	}
}

func TestEngineOneFails(t *testing.T) {
	env := agshctx.NewEnvelope("hello world", "text/plain", "test")
	intent := Intent{
		Description: "should contain missing text",
		Assertions: []Assertion{
			{Type: "not_empty", Target: "output"},
			{Type: "contains", Target: "output", Expected: "xyz-missing"},
			{Type: "contains", Target: "output", Expected: "hello"},
		},
	}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed {
		t.Error("expected overall failure")
	}
	// Without fail-fast, all 3 assertions should be checked.
	if len(result.Results) != 3 {
		t.Errorf("results count = %d, want 3", len(result.Results))
	}
	if result.Results[0].Passed != true {
		t.Error("first assertion should pass")
	}
	if result.Results[1].Passed != false {
		t.Error("second assertion should fail")
	}
	if result.Results[2].Passed != true {
		t.Error("third assertion should pass")
	}
}

func TestEngineFailFast(t *testing.T) {
	env := agshctx.NewEnvelope("hello", "text/plain", "test")
	intent := Intent{
		Description: "fail fast test",
		Assertions: []Assertion{
			{Type: "contains", Target: "output", Expected: "missing"},
			{Type: "not_empty", Target: "output"},
		},
	}

	engine := NewEngine(WithFailFast(true))
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed {
		t.Error("expected failure")
	}
	// Fail-fast: only the first assertion should be checked.
	if len(result.Results) != 1 {
		t.Errorf("results count = %d, want 1 (fail-fast)", len(result.Results))
	}
}

func TestEngineUnknownAssertionType(t *testing.T) {
	env := agshctx.NewEnvelope("hello", "text/plain", "test")
	intent := Intent{
		Description: "unknown type",
		Assertions: []Assertion{
			{Type: "nonexistent_check", Target: "output"},
		},
	}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed {
		t.Error("expected failure for unknown type")
	}
	if result.Results[0].Message == "" {
		t.Error("expected error message for unknown type")
	}
}

func TestEngineEmptyIntent(t *testing.T) {
	env := agshctx.NewEnvelope("hello", "text/plain", "test")
	intent := Intent{Description: "no assertions"}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed {
		t.Error("empty intent should pass")
	}
	if len(result.Results) != 0 {
		t.Errorf("results count = %d, want 0", len(result.Results))
	}
}

func TestVerifyEnvelopeConvenience(t *testing.T) {
	env := agshctx.NewEnvelope("test data", "text/plain", "test")
	intent := Intent{
		Description: "convenience function",
		Assertions: []Assertion{
			{Type: "not_empty", Target: "output"},
		},
	}

	result, err := VerifyEnvelope(env, intent)
	if err != nil {
		t.Fatalf("VerifyEnvelope: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass")
	}
}

func TestEngineMultipleAssertionTypes(t *testing.T) {
	env := agshctx.NewEnvelope(`{"name":"test","count":5}`, "application/json", "api")
	env.Meta.Tags["format"] = "json"

	intent := Intent{
		Description: "comprehensive check",
		Assertions: []Assertion{
			{Type: "not_empty", Target: "output"},
			{Type: "contains", Target: "output", Expected: "test"},
			{Type: "not_contains", Target: "output", Expected: "error"},
			{Type: "matches_regex", Target: "output", Expected: `"count":\d+`},
			{Type: "json_schema", Target: "output", Expected: map[string]any{
				"required": []any{"name", "count"},
			}},
			{Type: "contains", Target: "meta.tags.format", Expected: "json"},
		},
	}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed {
		for _, r := range result.Results {
			if !r.Passed {
				t.Errorf("assertion %s failed: %s", r.Assertion.Type, r.Message)
			}
		}
	}
}
