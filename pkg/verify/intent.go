package verify

import "time"

// Intent declares what a command or pipeline step is supposed to achieve.
type Intent struct {
	Description string      `json:"description"`
	Assertions  []Assertion `json:"assertions"`
}

// Assertion defines a machine-checkable condition.
type Assertion struct {
	Type     string `json:"type"`     // "not_empty", "contains", "not_contains", "count_gte", "matches_regex", "json_schema", "llm_judge"
	Target   string `json:"target"`   // what to check: "output", "output.lines", "meta.tags.y"
	Expected any    `json:"expected"` // the expected value/pattern
	Message  string `json:"message"`  // human-readable failure description
}

// VerificationResult holds the outcome of verifying an envelope against an intent.
type VerificationResult struct {
	Passed    bool              `json:"passed"`
	Results   []AssertionResult `json:"results"`
	Timestamp time.Time         `json:"timestamp"`
}

// AssertionResult records the outcome of a single assertion check.
type AssertionResult struct {
	Assertion Assertion `json:"assertion"`
	Passed    bool      `json:"passed"`
	Actual    any       `json:"actual"`
	Message   string    `json:"message"`
}
