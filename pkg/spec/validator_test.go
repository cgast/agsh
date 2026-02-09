package spec

import (
	"testing"
)

func validSpec() ProjectSpec {
	return ProjectSpec{
		APIVersion: "agsh/v1",
		Kind:       "ProjectSpec",
		Meta:       SpecMeta{Name: "test"},
		Goal:       "Do something",
		AllowedCommands: []string{"fs:*"},
		SuccessCriteria: []Assertion{
			{Type: "not_empty", Target: "output", Message: "check"},
		},
	}
}

func TestValidateSpecValid(t *testing.T) {
	result := ValidateSpec(validSpec())
	if !result.Valid() {
		t.Errorf("expected valid, got errors: %s", result.Error())
	}
}

func TestValidateSpecMissingAPIVersion(t *testing.T) {
	spec := validSpec()
	spec.APIVersion = ""
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for missing apiVersion")
	}
	assertHasFieldError(t, result, "apiVersion")
}

func TestValidateSpecBadAPIVersion(t *testing.T) {
	spec := validSpec()
	spec.APIVersion = "agsh/v99"
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for bad apiVersion")
	}
	assertHasFieldError(t, result, "apiVersion")
}

func TestValidateSpecMissingKind(t *testing.T) {
	spec := validSpec()
	spec.Kind = ""
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for missing kind")
	}
	assertHasFieldError(t, result, "kind")
}

func TestValidateSpecBadKind(t *testing.T) {
	spec := validSpec()
	spec.Kind = "NotASpec"
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for bad kind")
	}
	assertHasFieldError(t, result, "kind")
}

func TestValidateSpecMissingName(t *testing.T) {
	spec := validSpec()
	spec.Meta.Name = ""
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for missing meta.name")
	}
	assertHasFieldError(t, result, "meta.name")
}

func TestValidateSpecMissingGoal(t *testing.T) {
	spec := validSpec()
	spec.Goal = ""
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for missing goal")
	}
	assertHasFieldError(t, result, "goal")
}

func TestValidateSpecBadCommandPattern(t *testing.T) {
	spec := validSpec()
	spec.AllowedCommands = []string{"no-namespace"}
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for bad command pattern")
	}
	assertHasFieldError(t, result, "allowed_commands[0]")
}

func TestValidateSpecValidCommandPatterns(t *testing.T) {
	patterns := []string{"fs:*", "github:repo:info", "*", "http:get"}
	for _, p := range patterns {
		spec := validSpec()
		spec.AllowedCommands = []string{p}
		result := ValidateSpec(spec)
		if !result.Valid() {
			t.Errorf("pattern %q should be valid, got: %s", p, result.Error())
		}
	}
}

func TestValidateSpecBadAssertionType(t *testing.T) {
	spec := validSpec()
	spec.SuccessCriteria = []Assertion{
		{Type: "nonexistent_type", Target: "output"},
	}
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for bad assertion type")
	}
}

func TestValidateSpecDuplicateParams(t *testing.T) {
	spec := validSpec()
	spec.Params = []ParamDef{
		{Name: "days", Type: "integer"},
		{Name: "days", Type: "string"},
	}
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected validation error for duplicate param name")
	}
}

func TestValidateSpecMultipleErrors(t *testing.T) {
	spec := ProjectSpec{} // Everything missing.
	result := ValidateSpec(spec)
	if result.Valid() {
		t.Error("expected multiple validation errors")
	}
	if len(result.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %s", len(result.Errors), result.Error())
	}
}

func assertHasFieldError(t *testing.T, result ValidationResult, field string) {
	t.Helper()
	for _, e := range result.Errors {
		if e.Field == field {
			return
		}
	}
	t.Errorf("expected error for field %q, got errors: %v", field, result.Errors)
}
