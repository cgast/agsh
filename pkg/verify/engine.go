package verify

import (
	"fmt"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// VerificationEngine verifies envelopes against intents.
type VerificationEngine interface {
	Verify(envelope agshctx.Envelope, intent Intent) (VerificationResult, error)
}

// Option configures the DefaultEngine.
type Option func(*DefaultEngine)

// WithFailFast stops verification on the first failed assertion.
func WithFailFast(ff bool) Option {
	return func(e *DefaultEngine) {
		e.failFast = ff
	}
}

// DefaultEngine is the standard verification engine.
type DefaultEngine struct {
	failFast bool
}

// NewEngine creates a new verification engine with the given options.
func NewEngine(opts ...Option) *DefaultEngine {
	e := &DefaultEngine{}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Verify checks an envelope against all assertions in an intent.
func (e *DefaultEngine) Verify(envelope agshctx.Envelope, intent Intent) (VerificationResult, error) {
	result := VerificationResult{
		Passed:    true,
		Timestamp: time.Now(),
		Results:   make([]AssertionResult, 0, len(intent.Assertions)),
	}

	for _, assertion := range intent.Assertions {
		checker := GetChecker(assertion.Type)
		if checker == nil {
			ar := AssertionResult{
				Assertion: assertion,
				Passed:    false,
				Message:   fmt.Sprintf("unknown assertion type: %q", assertion.Type),
			}
			result.Results = append(result.Results, ar)
			result.Passed = false

			if e.failFast {
				return result, nil
			}
			continue
		}

		ar := checker(envelope, assertion)
		result.Results = append(result.Results, ar)

		if !ar.Passed {
			result.Passed = false
			if e.failFast {
				return result, nil
			}
		}
	}

	return result, nil
}

// VerifyEnvelope is a convenience function that creates a default engine and verifies.
func VerifyEnvelope(envelope agshctx.Envelope, intent Intent) (VerificationResult, error) {
	engine := NewEngine()
	return engine.Verify(envelope, intent)
}
