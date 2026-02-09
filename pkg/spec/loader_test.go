package spec

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.agsh.yaml")

	yamlData := `
apiVersion: agsh/v1
kind: ProjectSpec
meta:
  name: "test-project"
  description: "A test"
  author: "tester"
  created: "2025-02-09"
  tags: ["test"]
goal: "Do something"
constraints:
  - "Be fast"
success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Must not be empty"
allowed_commands:
  - "fs:*"
output:
  path: "./output.md"
  format: "markdown"
params:
  - name: "days"
    type: "integer"
    default: 7
    description: "Lookback"
`
	if err := os.WriteFile(path, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := LoadSpec(path, nil)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	if spec.Meta.Name != "test-project" {
		t.Errorf("Meta.Name = %q", spec.Meta.Name)
	}
	if spec.Goal != "Do something" {
		t.Errorf("Goal = %q", spec.Goal)
	}
	if len(spec.Params) != 1 {
		t.Errorf("Params len = %d", len(spec.Params))
	}
}

func TestLoadSpecMissing(t *testing.T) {
	_, err := LoadSpec("/nonexistent/spec.yaml", nil)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadSpecWithInterpolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.agsh.yaml")

	yamlData := `
apiVersion: agsh/v1
kind: ProjectSpec
meta:
  name: "report"
  description: "test"
  author: "tester"
  created: "2025-02-09"
goal: "Generate report"
output:
  path: "./reports/weekly-{{date}}.md"
  format: "markdown"
params:
  - name: "days"
    type: "integer"
    default: 7
    description: "Days back"
`
	if err := os.WriteFile(path, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := LoadSpec(path, nil)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	expected := "./reports/weekly-" + today + ".md"
	if spec.Output.Path != expected {
		t.Errorf("Output.Path = %q, want %q", spec.Output.Path, expected)
	}
}

func TestLoadSpecWithParamOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.agsh.yaml")

	yamlData := `
apiVersion: agsh/v1
kind: ProjectSpec
meta:
  name: "test"
  description: "test"
  author: "tester"
  created: "2025-02-09"
goal: "Test params"
output:
  path: "./out-{{days}}.md"
  format: "markdown"
params:
  - name: "days"
    type: "integer"
    default: 7
    description: "Lookback"
`
	if err := os.WriteFile(path, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	// Without override — uses default.
	spec, err := LoadSpec(path, nil)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Output.Path != "./out-7.md" {
		t.Errorf("Output.Path = %q, want %q", spec.Output.Path, "./out-7.md")
	}

	// With override.
	spec, err = LoadSpec(path, map[string]string{"days": "30"})
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Output.Path != "./out-30.md" {
		t.Errorf("Output.Path = %q, want %q", spec.Output.Path, "./out-30.md")
	}
}

func TestParseSpecInvalidYAML(t *testing.T) {
	_, err := ParseSpec([]byte("{{{{invalid yaml"), nil)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestInterpolateVars(t *testing.T) {
	vars := map[string]string{
		"date":  "2025-02-09",
		"name":  "alice",
		"count": "42",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"{{date}}", "2025-02-09"},
		{"hello {{name}}", "hello alice"},
		{"{{count}} items on {{date}}", "42 items on 2025-02-09"},
		{"{{unknown}}", "{{unknown}}"}, // unresolved stays
		{"no vars", "no vars"},
	}

	for _, tt := range tests {
		got := interpolateVars(tt.input, vars)
		if got != tt.want {
			t.Errorf("interpolateVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
