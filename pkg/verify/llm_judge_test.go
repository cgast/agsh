package verify

import (
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestLLMJudgeSkippedWhenNoEndpoint(t *testing.T) {
	// Ensure no endpoint is configured.
	oldEndpoint := LLMJudgeEndpoint
	LLMJudgeEndpoint = ""
	defer func() { LLMJudgeEndpoint = oldEndpoint }()

	env := agshctx.NewEnvelope("some output", "text/plain", "test")
	assertion := Assertion{
		Type:    "llm_judge",
		Target:  "output",
		Message: "output should be meaningful",
	}

	r := checkLLMJudge(env, assertion)
	if !r.Passed {
		t.Error("llm_judge should pass (skip) when no endpoint configured")
	}
	if r.Message == "" {
		t.Error("should have skip message")
	}
}

func TestLLMJudgeRegistered(t *testing.T) {
	checker := GetChecker("llm_judge")
	if checker == nil {
		t.Fatal("llm_judge should be registered")
	}
}

func TestLLMJudgeWithEndpoint(t *testing.T) {
	oldEndpoint := LLMJudgeEndpoint
	LLMJudgeEndpoint = "http://localhost:8080/judge"
	defer func() { LLMJudgeEndpoint = oldEndpoint }()

	env := agshctx.NewEnvelope("some output", "text/plain", "test")
	assertion := Assertion{
		Type:   "llm_judge",
		Target: "output",
	}

	r := checkLLMJudge(env, assertion)
	// With endpoint configured but no real service, should fail gracefully.
	if r.Passed {
		t.Error("llm_judge should fail when endpoint is set but not implemented")
	}
}

func TestLLMJudgeViaEngine(t *testing.T) {
	oldEndpoint := LLMJudgeEndpoint
	LLMJudgeEndpoint = ""
	defer func() { LLMJudgeEndpoint = oldEndpoint }()

	env := agshctx.NewEnvelope("hello", "text/plain", "test")
	intent := Intent{
		Description: "output should be a greeting",
		Assertions: []Assertion{
			{Type: "not_empty", Target: "output"},
			{Type: "llm_judge", Target: "output"},
		},
	}

	engine := NewEngine()
	result, err := engine.Verify(env, intent)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed {
		t.Error("should pass with not_empty + skipped llm_judge")
	}
}
