package verify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// AssertionChecker is a function that checks a single assertion against an envelope.
type AssertionChecker func(envelope agshctx.Envelope, assertion Assertion) AssertionResult

// builtinCheckers maps assertion type names to their checker implementations.
var builtinCheckers = map[string]AssertionChecker{
	"not_empty":     checkNotEmpty,
	"contains":      checkContains,
	"not_contains":  checkNotContains,
	"count_gte":     checkCountGTE,
	"matches_regex": checkMatchesRegex,
	"json_schema":   checkJSONSchema,
}

// RegisterChecker adds a custom assertion checker. Used for llm_judge etc.
func RegisterChecker(name string, checker AssertionChecker) {
	builtinCheckers[name] = checker
}

// GetChecker returns the checker for an assertion type, or nil if not found.
func GetChecker(name string) AssertionChecker {
	return builtinCheckers[name]
}

// resolveTarget extracts the value to check from the envelope based on the target string.
func resolveTarget(envelope agshctx.Envelope, target string) string {
	switch {
	case target == "" || target == "output":
		return envelope.PayloadString()
	case target == "output.lines":
		return envelope.PayloadString()
	case strings.HasPrefix(target, "meta.tags."):
		key := strings.TrimPrefix(target, "meta.tags.")
		if v, ok := envelope.Meta.Tags[key]; ok {
			return v
		}
		return ""
	case target == "meta.content_type":
		return envelope.Meta.ContentType
	case target == "meta.source":
		return envelope.Meta.Source
	default:
		return envelope.PayloadString()
	}
}

// checkNotEmpty verifies the target is not empty.
func checkNotEmpty(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	value := resolveTarget(envelope, assertion.Target)
	passed := envelope.Payload != nil && strings.TrimSpace(value) != "" && value != "null"
	msg := assertion.Message
	if !passed && msg == "" {
		msg = "output is empty"
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    passed,
		Actual:    value,
		Message:   msg,
	}
}

// checkContains verifies the target contains the expected substring.
func checkContains(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	value := resolveTarget(envelope, assertion.Target)
	expected := fmt.Sprintf("%v", assertion.Expected)
	passed := strings.Contains(value, expected)
	msg := assertion.Message
	if !passed && msg == "" {
		msg = fmt.Sprintf("output does not contain %q", expected)
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    passed,
		Actual:    truncate(value, 200),
		Message:   msg,
	}
}

// checkNotContains verifies the target does NOT contain the expected substring.
func checkNotContains(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	value := resolveTarget(envelope, assertion.Target)
	expected := fmt.Sprintf("%v", assertion.Expected)
	passed := !strings.Contains(value, expected)
	msg := assertion.Message
	if !passed && msg == "" {
		msg = fmt.Sprintf("output should not contain %q", expected)
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    passed,
		Actual:    truncate(value, 200),
		Message:   msg,
	}
}

// checkCountGTE verifies that the line count (or array length) is >= expected.
func checkCountGTE(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	expected, err := toInt(assertion.Expected)
	if err != nil {
		return AssertionResult{
			Assertion: assertion,
			Passed:    false,
			Message:   fmt.Sprintf("count_gte: invalid expected value: %v", assertion.Expected),
		}
	}

	var actual int
	if assertion.Target == "output.lines" {
		// Count non-empty lines.
		value := resolveTarget(envelope, assertion.Target)
		lines := strings.Split(value, "\n")
		actual = len(lines)
	} else {
		// Try as array payload.
		switch v := envelope.Payload.(type) {
		case []any:
			actual = len(v)
		case []string:
			actual = len(v)
		default:
			// Fall back to line count of string representation.
			value := resolveTarget(envelope, assertion.Target)
			actual = len(strings.Split(value, "\n"))
		}
	}

	passed := actual >= expected
	msg := assertion.Message
	if !passed && msg == "" {
		msg = fmt.Sprintf("count %d is less than expected %d", actual, expected)
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    passed,
		Actual:    actual,
		Message:   msg,
	}
}

// checkMatchesRegex verifies the target matches the expected regex pattern.
func checkMatchesRegex(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	value := resolveTarget(envelope, assertion.Target)
	pattern := fmt.Sprintf("%v", assertion.Expected)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return AssertionResult{
			Assertion: assertion,
			Passed:    false,
			Message:   fmt.Sprintf("matches_regex: invalid pattern %q: %v", pattern, err),
		}
	}

	passed := re.MatchString(value)
	msg := assertion.Message
	if !passed && msg == "" {
		msg = fmt.Sprintf("output does not match regex %q", pattern)
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    passed,
		Actual:    truncate(value, 200),
		Message:   msg,
	}
}

// checkJSONSchema verifies the output is valid JSON matching basic structural requirements.
// For the prototype, this checks that the output is valid JSON and optionally
// that required keys exist.
func checkJSONSchema(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	value := resolveTarget(envelope, assertion.Target)

	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return AssertionResult{
			Assertion: assertion,
			Passed:    false,
			Actual:    truncate(value, 200),
			Message:   fmt.Sprintf("json_schema: not valid JSON: %v", err),
		}
	}

	// If expected is a map with "required" keys, check they exist.
	if schema, ok := assertion.Expected.(map[string]any); ok {
		if required, ok := schema["required"].([]any); ok {
			if obj, ok := parsed.(map[string]any); ok {
				for _, key := range required {
					keyStr := fmt.Sprintf("%v", key)
					if _, exists := obj[keyStr]; !exists {
						return AssertionResult{
							Assertion: assertion,
							Passed:    false,
							Actual:    parsed,
							Message:   fmt.Sprintf("json_schema: missing required key %q", keyStr),
						}
					}
				}
			}
		}
	}

	msg := assertion.Message
	if msg == "" {
		msg = "JSON is valid"
	}
	return AssertionResult{
		Assertion: assertion,
		Passed:    true,
		Actual:    parsed,
		Message:   msg,
	}
}

// toInt converts various numeric types to int.
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		var i int
		_, err := fmt.Sscanf(n, "%d", &i)
		return i, err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
