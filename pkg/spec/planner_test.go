package spec

import (
	"strings"
	"testing"
)

// mockLister implements CommandLister for testing.
type mockLister struct {
	names []string
}

func (m *mockLister) Names() []string { return m.names }

func (m *mockLister) MatchGlob(pattern string) []string {
	if pattern == "*" {
		return m.names
	}
	prefix := strings.TrimSuffix(pattern, "*")
	var result []string
	for _, name := range m.names {
		if strings.HasPrefix(name, prefix) {
			result = append(result, name)
		}
	}
	return result
}

func TestGeneratePlan(t *testing.T) {
	spec := ProjectSpec{
		APIVersion: "agsh/v1",
		Kind:       "ProjectSpec",
		Meta:       SpecMeta{Name: "test-plan"},
		Goal:       "Test plan generation",
		AllowedCommands: []string{"fs:*"},
		Output: OutputSpec{
			Path:   "./output.md",
			Format: "markdown",
		},
		SuccessCriteria: []Assertion{
			{Type: "not_empty", Target: "output", Message: "check"},
		},
	}

	lister := &mockLister{
		names: []string{"fs:list", "fs:read", "fs:write"},
	}

	plan, err := GeneratePlan(spec, lister)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	if plan.Spec != "test-plan" {
		t.Errorf("Spec = %q", plan.Spec)
	}

	if len(plan.Steps) == 0 {
		t.Fatal("expected at least one step")
	}

	// Check that read steps come before write steps.
	foundWrite := false
	for _, step := range plan.Steps {
		if step.Risk == "write" {
			foundWrite = true
		}
		if step.Risk == "read-only" && foundWrite {
			t.Error("read-only step after write step")
		}
	}

	// Check that write steps have checkpoint.
	for _, step := range plan.Steps {
		if step.Risk == "write" && !step.CheckpointBefore {
			t.Errorf("write step %q should have checkpoint", step.Command)
		}
	}

	// Check that fs:write step has output path.
	var writeStep *PlanStep
	for i, step := range plan.Steps {
		if step.Command == "fs:write" {
			writeStep = &plan.Steps[i]
			break
		}
	}
	if writeStep == nil {
		t.Fatal("expected fs:write step")
	}
	if len(writeStep.Args) == 0 || writeStep.Args[0] != "./output.md" {
		t.Errorf("fs:write step args = %v, want [./output.md]", writeStep.Args)
	}

	if len(plan.SuccessCriteria) != 1 {
		t.Errorf("SuccessCriteria len = %d, want 1", len(plan.SuccessCriteria))
	}
}

func TestGeneratePlanInvalidSpec(t *testing.T) {
	spec := ProjectSpec{} // invalid
	_, err := GeneratePlan(spec, nil)
	if err == nil {
		t.Error("expected error for invalid spec")
	}
}

func TestGeneratePlanNilLister(t *testing.T) {
	spec := ProjectSpec{
		APIVersion:      "agsh/v1",
		Kind:            "ProjectSpec",
		Meta:            SpecMeta{Name: "test"},
		Goal:            "Test",
		AllowedCommands: []string{"fs:list", "fs:write"},
	}

	plan, err := GeneratePlan(spec, nil)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	// Without lister, patterns pass through as-is.
	if len(plan.AllowedCommands) != 2 {
		t.Errorf("AllowedCommands = %v", plan.AllowedCommands)
	}
}

func TestGeneratePlanGitHubReport(t *testing.T) {
	spec := ProjectSpec{
		APIVersion: "agsh/v1",
		Kind:       "ProjectSpec",
		Meta:       SpecMeta{Name: "github-weekly-report"},
		Goal:       "Generate a weekly GitHub report",
		AllowedCommands: []string{
			"github:repo:list",
			"github:pr:list",
			"github:issue:list",
			"fs:write",
		},
		Output: OutputSpec{
			Path:   "./reports/weekly.md",
			Format: "markdown",
		},
	}

	plan, err := GeneratePlan(spec, nil)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	// Should have 3 read steps and 1 write step.
	readCount := 0
	writeCount := 0
	for _, step := range plan.Steps {
		switch step.Risk {
		case "read-only":
			readCount++
		case "write":
			writeCount++
		}
	}
	if readCount != 3 {
		t.Errorf("read steps = %d, want 3", readCount)
	}
	if writeCount != 1 {
		t.Errorf("write steps = %d, want 1", writeCount)
	}

	if !strings.Contains(plan.EstimatedRisk, "3 read-only") {
		t.Errorf("risk summary = %q, expected '3 read-only'", plan.EstimatedRisk)
	}
}

func TestResolveAllowedCommands(t *testing.T) {
	lister := &mockLister{
		names: []string{
			"fs:list", "fs:read", "fs:write",
			"github:repo:info", "github:pr:list", "github:issue:create",
			"http:get", "http:post",
		},
	}

	tests := []struct {
		name     string
		patterns []string
		want     int
	}{
		{"wildcard all", []string{"*"}, 8},
		{"fs glob", []string{"fs:*"}, 3},
		{"github glob", []string{"github:*"}, 3},
		{"exact match", []string{"fs:list"}, 1},
		{"mixed", []string{"fs:*", "github:repo:info"}, 4},
		{"dedup", []string{"fs:*", "fs:list"}, 3}, // fs:list already in fs:*
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveAllowedCommands(tt.patterns, lister)
			if len(result) != tt.want {
				t.Errorf("got %d commands %v, want %d", len(result), result, tt.want)
			}
		})
	}
}

func TestIsWriteCommand(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"fs:list", false},
		{"fs:read", false},
		{"fs:write", true},
		{"github:repo:info", false},
		{"github:pr:list", false},
		{"github:issue:create", true},
		{"http:get", false},
		{"http:post", true},
	}

	for _, tt := range tests {
		got := isWriteCommand(tt.name)
		if got != tt.want {
			t.Errorf("isWriteCommand(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyCommands(t *testing.T) {
	commands := []string{"fs:list", "fs:read", "fs:write", "github:pr:list", "github:issue:create"}
	reads, writes := classifyCommands(commands)

	if len(reads) != 3 {
		t.Errorf("reads = %v, want 3", reads)
	}
	if len(writes) != 2 {
		t.Errorf("writes = %v, want 2", writes)
	}
}
