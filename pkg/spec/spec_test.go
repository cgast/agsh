package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProjectSpecRoundTrip(t *testing.T) {
	original := ProjectSpec{
		APIVersion: "agsh/v1",
		Kind:       "ProjectSpec",
		Meta: SpecMeta{
			Name:        "test-spec",
			Description: "A test specification",
			Author:      "test",
			Created:     "2025-02-09",
			Tags:        []string{"test", "demo"},
		},
		Goal:        "Do something useful",
		Constraints: []string{"Be fast", "Be correct"},
		Guidelines:  []string{"Use markdown"},
		SuccessCriteria: []Assertion{
			{Type: "not_empty", Target: "output", Message: "Output must not be empty"},
			{Type: "contains", Target: "output", Expected: "## ", Message: "Must have headers"},
		},
		AllowedCommands: []string{"fs:*", "github:repo:info"},
		Output: OutputSpec{
			Path:   "./output.md",
			Format: "markdown",
		},
		Params: []ParamDef{
			{Name: "days", Type: "integer", Default: 7, Description: "Lookback days"},
		},
	}

	// Marshal to YAML.
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Unmarshal back.
	var parsed ProjectSpec
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if parsed.APIVersion != original.APIVersion {
		t.Errorf("APIVersion = %q, want %q", parsed.APIVersion, original.APIVersion)
	}
	if parsed.Kind != original.Kind {
		t.Errorf("Kind = %q, want %q", parsed.Kind, original.Kind)
	}
	if parsed.Meta.Name != original.Meta.Name {
		t.Errorf("Meta.Name = %q, want %q", parsed.Meta.Name, original.Meta.Name)
	}
	if parsed.Goal != original.Goal {
		t.Errorf("Goal = %q, want %q", parsed.Goal, original.Goal)
	}
	if len(parsed.Constraints) != len(original.Constraints) {
		t.Errorf("Constraints len = %d, want %d", len(parsed.Constraints), len(original.Constraints))
	}
	if len(parsed.SuccessCriteria) != 2 {
		t.Errorf("SuccessCriteria len = %d, want 2", len(parsed.SuccessCriteria))
	}
	if parsed.SuccessCriteria[0].Type != "not_empty" {
		t.Errorf("SuccessCriteria[0].Type = %q, want %q", parsed.SuccessCriteria[0].Type, "not_empty")
	}
	if len(parsed.AllowedCommands) != 2 {
		t.Errorf("AllowedCommands len = %d, want 2", len(parsed.AllowedCommands))
	}
	if parsed.Output.Path != original.Output.Path {
		t.Errorf("Output.Path = %q, want %q", parsed.Output.Path, original.Output.Path)
	}
	if len(parsed.Params) != 1 {
		t.Errorf("Params len = %d, want 1", len(parsed.Params))
	}
	if parsed.Params[0].Name != "days" {
		t.Errorf("Params[0].Name = %q, want %q", parsed.Params[0].Name, "days")
	}
}

func TestProjectSpecFromYAML(t *testing.T) {
	yamlData := `
apiVersion: agsh/v1
kind: ProjectSpec
meta:
  name: "heading-counter"
  description: "Count markdown headings"
  author: "demo"
  created: "2025-02-09"
  tags: ["demo", "basic"]
goal: |
  Count headings in markdown files.
constraints:
  - "Read-only"
guidelines:
  - "Sort alphabetically"
success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Must not be empty"
  - type: "contains"
    target: "output"
    expected: "## "
    message: "Must have headers"
allowed_commands:
  - "fs:*"
output:
  path: "./output.md"
  format: "markdown"
params: []
`
	var spec ProjectSpec
	if err := yaml.Unmarshal([]byte(yamlData), &spec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if spec.APIVersion != "agsh/v1" {
		t.Errorf("APIVersion = %q", spec.APIVersion)
	}
	if spec.Meta.Name != "heading-counter" {
		t.Errorf("Meta.Name = %q", spec.Meta.Name)
	}
	if len(spec.Meta.Tags) != 2 {
		t.Errorf("Meta.Tags = %v", spec.Meta.Tags)
	}
	if len(spec.SuccessCriteria) != 2 {
		t.Errorf("SuccessCriteria len = %d", len(spec.SuccessCriteria))
	}
	if spec.SuccessCriteria[1].Expected != "## " {
		t.Errorf("SuccessCriteria[1].Expected = %v", spec.SuccessCriteria[1].Expected)
	}
}
