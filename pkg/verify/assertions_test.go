package verify

import (
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func envelope(payload any) agshctx.Envelope {
	return agshctx.NewEnvelope(payload, "text/plain", "test")
}

func TestCheckNotEmpty(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    bool
	}{
		{"non-empty string", "hello", true},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"nil payload", nil, false},
		{"map payload", map[string]any{"key": "val"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkNotEmpty(envelope(tt.payload), Assertion{Type: "not_empty", Target: "output"})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.want)
			}
		})
	}
}

func TestCheckContains(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected string
		want     bool
	}{
		{"contains substring", "hello world", "world", true},
		{"missing substring", "hello world", "foo", false},
		{"empty expected", "hello", "", true},
		{"markdown header", "# Title\n## Section", "## ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkContains(envelope(tt.payload), Assertion{Type: "contains", Target: "output", Expected: tt.expected})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.want)
			}
		})
	}
}

func TestCheckNotContains(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected string
		want     bool
	}{
		{"does not contain", "hello world", "foo", true},
		{"contains it", "hello world", "world", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkNotContains(envelope(tt.payload), Assertion{Type: "not_contains", Target: "output", Expected: tt.expected})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.want)
			}
		})
	}
}

func TestCheckCountGTE(t *testing.T) {
	tests := []struct {
		name     string
		payload  any
		target   string
		expected any
		want     bool
	}{
		{"lines >= 3", "line1\nline2\nline3", "output.lines", 3, true},
		{"lines < 5", "line1\nline2", "output.lines", 5, false},
		{"array >= 2", []any{"a", "b", "c"}, "output", 2, true},
		{"array < 5", []any{"a"}, "output", 5, false},
		{"float expected", "a\nb\nc", "output.lines", 3.0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkCountGTE(envelope(tt.payload), Assertion{Type: "count_gte", Target: tt.target, Expected: tt.expected})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v (actual=%v)", r.Passed, tt.want, r.Actual)
			}
		})
	}
}

func TestCheckCountGTEInvalidExpected(t *testing.T) {
	r := checkCountGTE(envelope("hello"), Assertion{Type: "count_gte", Expected: "not-a-number"})
	if r.Passed {
		t.Error("should fail for invalid expected")
	}
}

func TestCheckMatchesRegex(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected string
		want     bool
	}{
		{"matches digits", "abc123", `\d+`, true},
		{"no match", "abcdef", `\d+`, false},
		{"matches pipe table", "| Name | Age |", `\| Name\s*\|`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkMatchesRegex(envelope(tt.payload), Assertion{Type: "matches_regex", Target: "output", Expected: tt.expected})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.want)
			}
		})
	}
}

func TestCheckMatchesRegexInvalid(t *testing.T) {
	r := checkMatchesRegex(envelope("hello"), Assertion{Type: "matches_regex", Expected: "[invalid"})
	if r.Passed {
		t.Error("should fail for invalid regex")
	}
}

func TestCheckJSONSchema(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected any
		want     bool
	}{
		{"valid JSON object", `{"name":"test"}`, nil, true},
		{"valid JSON array", `[1,2,3]`, nil, true},
		{"invalid JSON", `not json`, nil, false},
		{
			"with required keys - pass",
			`{"name":"test","age":25}`,
			map[string]any{"required": []any{"name", "age"}},
			true,
		},
		{
			"with required keys - fail",
			`{"name":"test"}`,
			map[string]any{"required": []any{"name", "age"}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkJSONSchema(envelope(tt.payload), Assertion{Type: "json_schema", Target: "output", Expected: tt.expected})
			if r.Passed != tt.want {
				t.Errorf("Passed = %v, want %v (msg=%s)", r.Passed, tt.want, r.Message)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	env := agshctx.NewEnvelope("payload-data", "text/plain", "test-source")
	env.Meta.Tags["format"] = "markdown"

	tests := []struct {
		target string
		want   string
	}{
		{"output", "payload-data"},
		{"", "payload-data"},
		{"output.lines", "payload-data"},
		{"meta.tags.format", "markdown"},
		{"meta.tags.missing", ""},
		{"meta.content_type", "text/plain"},
		{"meta.source", "test-source"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := resolveTarget(env, tt.target)
			if got != tt.want {
				t.Errorf("resolveTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("should not truncate short string")
	}
	if truncate("this is a long string", 10) != "this is a ..." {
		t.Errorf("truncated = %q", truncate("this is a long string", 10))
	}
}
