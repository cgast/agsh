package main

import (
	"bufio"
	"encoding/json"
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/spec"
)

// registryLister adapts platform.Registry to spec.CommandLister.
type registryLister struct {
	registry *platform.Registry
}

func (l *registryLister) Names() []string {
	return l.registry.Names()
}

func (l *registryLister) MatchGlob(pattern string) []string {
	cmds := l.registry.MatchGlob(pattern)
	names := make([]string, len(cmds))
	for i, cmd := range cmds {
		names[i] = cmd.Name()
	}
	return names
}

// handleRun implements `agsh run <spec.yaml> [--param key=value ...]`.
func handleRun(registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus) error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agsh run <spec.yaml> [--param key=value ...]")
		return nil
	}

	specPath := os.Args[2]
	params := parseRunParams(os.Args[3:])

	// Load and validate spec.
	fmt.Fprintf(os.Stderr, "Loading spec: %s\n", specPath)
	projSpec, err := spec.LoadSpec(specPath, params)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	vr := spec.ValidateSpec(projSpec)
	if !vr.Valid() {
		return fmt.Errorf("spec validation failed:\n  %s", strings.Join(validationMessages(vr), "\n  "))
	}

	fmt.Fprintf(os.Stderr, "Spec: %s — %s\n", projSpec.Meta.Name, projSpec.Meta.Description)
	fmt.Fprintf(os.Stderr, "Goal: %s\n", strings.TrimSpace(projSpec.Goal))

	// Generate plan.
	lister := &registryLister{registry: registry}
	plan, err := spec.GeneratePlan(projSpec, lister)
	if err != nil {
		return fmt.Errorf("generate plan: %w", err)
	}

	// Display plan.
	fmt.Fprintf(os.Stderr, "\n=== Execution Plan ===\n")
	displayPlan(plan)

	// Ask for approval (interactive only).
	if !approveExecution() {
		fmt.Fprintln(os.Stderr, "Execution cancelled.")
		return nil
	}

	// Execute the plan as a pipeline.
	fmt.Fprintf(os.Stderr, "\n=== Executing ===\n")
	return executePlan(plan, registry, store, bus)
}

// parseRunParams extracts --param key=value pairs from args.
func parseRunParams(args []string) map[string]string {
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if args[i] == "--param" && i+1 < len(args) {
			i++
			if k, v, ok := strings.Cut(args[i], "="); ok {
				params[k] = v
			}
		} else if strings.HasPrefix(args[i], "--param=") {
			rest := strings.TrimPrefix(args[i], "--param=")
			if k, v, ok := strings.Cut(rest, "="); ok {
				params[k] = v
			}
		}
	}
	return params
}

// displayPlan prints a human-readable representation of the execution plan.
func displayPlan(plan spec.ExecutionPlan) {
	fmt.Fprintf(os.Stderr, "Spec: %s\n", plan.Spec)
	fmt.Fprintf(os.Stderr, "Risk: %s\n", plan.EstimatedRisk)
	fmt.Fprintf(os.Stderr, "Steps:\n")
	for i, step := range plan.Steps {
		checkpoint := ""
		if step.CheckpointBefore {
			checkpoint = " [checkpoint]"
		}
		args := ""
		if len(step.Args) > 0 {
			args = " " + strings.Join(step.Args, " ")
		}
		fmt.Fprintf(os.Stderr, "  %d. %s%s (%s)%s\n", i+1, step.Command, args, step.Risk, checkpoint)
		fmt.Fprintf(os.Stderr, "     Intent: %s\n", step.Intent)
	}
	if len(plan.SuccessCriteria) > 0 {
		fmt.Fprintf(os.Stderr, "Success criteria: %d assertion(s)\n", len(plan.SuccessCriteria))
	}
	if plan.Output.Path != "" {
		fmt.Fprintf(os.Stderr, "Output: %s (%s)\n", plan.Output.Path, plan.Output.Format)
	}
}

// approveExecution asks the user to approve before executing.
func approveExecution() bool {
	fmt.Fprintf(os.Stderr, "\nProceed with execution? [Y/n] ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}

// executePlan runs an ExecutionPlan through the pipeline engine.
func executePlan(plan spec.ExecutionPlan, registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus) error {
	executor := &registryExecutor{registry: registry}
	publisher := &eventBusPublisher{bus: bus}

	// Convert plan steps to pipeline steps.
	pipelineSteps := make([]agshctx.PipelineStep, len(plan.Steps))
	for i, step := range plan.Steps {
		pipelineSteps[i] = agshctx.PipelineStep{
			Command: step.Command,
			Args:    step.Args,
			Intent:  step.Intent,
			OnError: step.OnError,
		}
	}

	// Store spec info in project context.
	store.Set(agshctx.ScopeProject, "spec_name", plan.Spec)
	store.Set(agshctx.ScopeProject, "output_path", plan.Output.Path)

	pipeline := &agshctx.Pipeline{
		Steps:    pipelineSteps,
		Context:  store,
		Executor: executor,
		Events:   publisher,
	}

	ctx := gocontext.Background()
	input := agshctx.NewEnvelope(nil, "text/plain", "run")

	result, err := pipeline.Run(ctx, input)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Display result.
	fmt.Fprintf(os.Stderr, "\n=== Result ===\n")
	if result.Success {
		fmt.Fprintf(os.Stderr, "Execution completed successfully (%d steps)\n", len(result.Steps))
	} else {
		fmt.Fprintf(os.Stderr, "Execution completed with errors\n")
	}

	// Print the final output.
	output, err := json.MarshalIndent(result.Output.Payload, "", "  ")
	if err != nil {
		fmt.Println(result.Output.PayloadString())
	} else {
		fmt.Println(string(output))
	}

	return nil
}

// validationMessages extracts messages from a ValidationResult.
func validationMessages(vr spec.ValidationResult) []string {
	msgs := make([]string, len(vr.Errors))
	for i, e := range vr.Errors {
		msgs[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return msgs
}

// handleValidate implements `agsh validate <spec.yaml>`.
func handleValidate() error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agsh validate <spec.yaml>")
		return nil
	}

	specPath := os.Args[2]
	projSpec, err := spec.LoadSpec(specPath, nil)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	vr := spec.ValidateSpec(projSpec)
	if vr.Valid() {
		fmt.Printf("Spec %q is valid.\n", projSpec.Meta.Name)
		return nil
	}

	fmt.Printf("Spec %q has %d error(s):\n", filepath.Base(specPath), len(vr.Errors))
	for _, e := range vr.Errors {
		fmt.Printf("  - %s: %s\n", e.Field, e.Message)
	}
	return fmt.Errorf("validation failed")
}
