package spec

import (
	"fmt"
	"strings"
)

// ValidationError represents a single validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds all validation errors for a spec.
type ValidationResult struct {
	Errors []ValidationError
}

// Valid returns true if no validation errors were found.
func (r ValidationResult) Valid() bool {
	return len(r.Errors) == 0
}

// Error returns a combined error message from all validation errors.
func (r ValidationResult) Error() string {
	if r.Valid() {
		return ""
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Error()
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(msgs, "; "))
}

// ValidateSpec checks a ProjectSpec for required fields and structural correctness.
func ValidateSpec(spec ProjectSpec) ValidationResult {
	var result ValidationResult

	// Required fields.
	if spec.APIVersion == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "apiVersion", Message: "required",
		})
	} else if spec.APIVersion != "agsh/v1" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "apiVersion", Message: fmt.Sprintf("unsupported version %q (expected agsh/v1)", spec.APIVersion),
		})
	}

	if spec.Kind == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "kind", Message: "required",
		})
	} else if spec.Kind != "ProjectSpec" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "kind", Message: fmt.Sprintf("unsupported kind %q (expected ProjectSpec)", spec.Kind),
		})
	}

	if spec.Meta.Name == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "meta.name", Message: "required",
		})
	}

	if strings.TrimSpace(spec.Goal) == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field: "goal", Message: "required",
		})
	}

	// Validate allowed_commands patterns.
	for i, pattern := range spec.AllowedCommands {
		if err := validateCommandPattern(pattern); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("allowed_commands[%d]", i),
				Message: err.Error(),
			})
		}
	}

	// Validate success_criteria assertions.
	for i, a := range spec.SuccessCriteria {
		if a.Type == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("success_criteria[%d].type", i),
				Message: "required",
			})
		} else if !isValidAssertionType(a.Type) {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("success_criteria[%d].type", i),
				Message: fmt.Sprintf("unknown assertion type %q", a.Type),
			})
		}
	}

	// Validate params.
	paramNames := make(map[string]bool)
	for i, p := range spec.Params {
		if p.Name == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("params[%d].name", i),
				Message: "required",
			})
		} else if paramNames[p.Name] {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("params[%d].name", i),
				Message: fmt.Sprintf("duplicate param name %q", p.Name),
			})
		} else {
			paramNames[p.Name] = true
		}
	}

	return result
}

// validAssertionTypes lists the recognized assertion types.
var validAssertionTypes = map[string]bool{
	"not_empty":     true,
	"contains":      true,
	"not_contains":  true,
	"count_gte":     true,
	"json_schema":   true,
	"matches_regex": true,
	"llm_judge":     true,
}

func isValidAssertionType(t string) bool {
	return validAssertionTypes[t]
}

// validateCommandPattern checks that a command glob pattern is well-formed.
// Patterns must be non-empty and use the format "namespace:command" or "namespace:*".
func validateCommandPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty command pattern")
	}
	if pattern == "*" {
		return nil // Wildcard for everything.
	}
	if !strings.Contains(pattern, ":") {
		return fmt.Errorf("invalid pattern %q (expected namespace:command format)", pattern)
	}
	parts := strings.SplitN(pattern, ":", 2)
	if parts[0] == "" {
		return fmt.Errorf("invalid pattern %q (empty namespace)", pattern)
	}
	return nil
}
