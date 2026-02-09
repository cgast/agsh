package spec

import (
	"fmt"
	"strings"
)

// CommandLister provides the list of available commands for plan validation.
// This avoids importing pkg/platform directly.
type CommandLister interface {
	Names() []string
	MatchGlob(pattern string) []string
}

// ExecutionPlan is the concrete plan generated from a ProjectSpec.
type ExecutionPlan struct {
	Spec            string        `json:"spec"`
	Steps           []PlanStep    `json:"steps"`
	EstimatedRisk   string        `json:"risk_summary"`
	AllowedCommands []string      `json:"allowed_commands"`
	SuccessCriteria []Assertion   `json:"success_criteria,omitempty"`
	Output          OutputSpec    `json:"output"`
}

// PlanStep is a single step in an execution plan.
type PlanStep struct {
	Command          string   `json:"command"`
	Args             []string `json:"args,omitempty"`
	Intent           string   `json:"intent"`
	Risk             string   `json:"risk"`                        // "read-only", "write", "destructive"
	CheckpointBefore bool     `json:"checkpoint_before,omitempty"`
	OnError          string   `json:"on_error"`                    // "stop", "skip", "retry"
}

// GeneratePlan produces an ExecutionPlan from a validated ProjectSpec.
// The plan is a structured preview of what will be executed, suitable for
// human review before execution.
func GeneratePlan(spec ProjectSpec, lister CommandLister) (ExecutionPlan, error) {
	// Validate spec first.
	vr := ValidateSpec(spec)
	if !vr.Valid() {
		return ExecutionPlan{}, fmt.Errorf("invalid spec: %s", vr.Error())
	}

	// Resolve which commands are available and allowed.
	available := resolveAllowedCommands(spec.AllowedCommands, lister)

	// Classify risk levels.
	reads, writes := classifyCommands(available)

	// Build plan steps.
	steps := buildSteps(spec, reads, writes)

	riskSummary := fmt.Sprintf("%d read-only, %d write operations", len(reads), len(writes))

	return ExecutionPlan{
		Spec:            spec.Meta.Name,
		Steps:           steps,
		EstimatedRisk:   riskSummary,
		AllowedCommands: available,
		SuccessCriteria: spec.SuccessCriteria,
		Output:          spec.Output,
	}, nil
}

// resolveAllowedCommands expands glob patterns in allowed_commands against
// the available commands in the registry. If no lister is provided, returns
// the patterns as-is.
func resolveAllowedCommands(patterns []string, lister CommandLister) []string {
	if lister == nil {
		return patterns
	}

	seen := make(map[string]bool)
	var result []string

	for _, pattern := range patterns {
		if strings.Contains(pattern, "*") {
			for _, name := range lister.MatchGlob(pattern) {
				if !seen[name] {
					seen[name] = true
					result = append(result, name)
				}
			}
		} else {
			if !seen[pattern] {
				seen[pattern] = true
				result = append(result, pattern)
			}
		}
	}
	return result
}

// classifyCommands separates commands into read-only and write operations
// based on naming conventions.
func classifyCommands(commands []string) (reads, writes []string) {
	for _, cmd := range commands {
		if isWriteCommand(cmd) {
			writes = append(writes, cmd)
		} else {
			reads = append(reads, cmd)
		}
	}
	return
}

// isWriteCommand determines if a command is a write operation based on naming.
var writeVerbs = []string{"write", "create", "delete", "update", "post", "put", "patch"}

func isWriteCommand(name string) bool {
	lower := strings.ToLower(name)
	for _, verb := range writeVerbs {
		if strings.Contains(lower, verb) {
			return true
		}
	}
	return false
}

// buildSteps creates plan steps from the spec's goal and allowed commands.
// The planner uses heuristics based on the spec structure to produce a
// reasonable execution plan.
func buildSteps(spec ProjectSpec, reads, writes []string) []PlanStep {
	var steps []PlanStep

	// Add read steps for data gathering.
	for _, cmd := range reads {
		steps = append(steps, PlanStep{
			Command: cmd,
			Intent:  fmt.Sprintf("Gather data using %s", cmd),
			Risk:    "read-only",
			OnError: "stop",
		})
	}

	// Add write steps with checkpoints.
	for _, cmd := range writes {
		step := PlanStep{
			Command:          cmd,
			Intent:           fmt.Sprintf("Write output using %s", cmd),
			Risk:             "write",
			CheckpointBefore: true,
			OnError:          "stop",
		}

		// If output path is specified, add it as an arg for fs:write.
		if cmd == "fs:write" && spec.Output.Path != "" {
			step.Args = []string{spec.Output.Path}
			step.Intent = fmt.Sprintf("Write final output to %s", spec.Output.Path)
		}

		steps = append(steps, step)
	}

	return steps
}
