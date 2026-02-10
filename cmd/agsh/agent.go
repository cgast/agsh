package main

import (
	"bufio"
	"encoding/json"
	gocontext "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/protocol"
	"github.com/cgast/agsh/pkg/spec"
	"github.com/cgast/agsh/pkg/verify"
)

// agentState holds the mutable state for agent mode sessions.
type agentState struct {
	mu          sync.Mutex
	loadedSpec  *spec.ProjectSpec
	pendingPlan *spec.ExecutionPlan
	planID      string
}

// runAgentMode starts the JSON-RPC agent mode loop on stdin/stdout.
func runAgentMode(registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus) {
	handler := protocol.NewHandler()
	state := &agentState{}

	// Set up checkpoint manager.
	cpDir := filepath.Join(os.TempDir(), "agsh-agent-checkpoints")
	cpMgr, _ := verify.NewFileCheckpointManager(cpDir)

	// Register all methods.
	registerCoreMethods(handler, registry, store, bus, cpMgr)
	registerProjectMethods(handler, registry, store, bus, state, cpMgr)

	// Emit agent start event.
	bus.Publish(events.NewEvent(events.EventAgentMessage, map[string]any{
		"message": "agent mode started",
		"methods": handler.Methods(),
	}))

	// Read JSON-RPC requests from stdin, write responses to stdout.
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB max line

	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		resp := handler.HandleRaw([]byte(line))
		if err := encoder.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding response: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin read error: %v\n", err)
	}
}

// registerCoreMethods registers the base set of JSON-RPC methods.
func registerCoreMethods(h *protocol.Handler, registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus, cpMgr verify.CheckpointManager) {
	// commands.list
	h.Register(protocol.MethodCommandsList, func(params json.RawMessage) (any, *protocol.Error) {
		cmds := registry.List("")
		infos := make([]protocol.CommandInfo, len(cmds))
		for i, cmd := range cmds {
			infos[i] = protocol.CommandInfo{
				Name:        cmd.Name(),
				Description: cmd.Description(),
				Namespace:   cmd.Namespace(),
			}
		}
		return infos, nil
	})

	// commands.describe
	h.Register(protocol.MethodCommandsDescribe, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.CommandsDescribeParams](params)
		if err != nil {
			return nil, err
		}
		cmd, resolveErr := registry.Resolve(p.Name)
		if resolveErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeCommandNotFound, Message: resolveErr.Error()}
		}
		inSchema := cmd.InputSchema()
		outSchema := cmd.OutputSchema()
		return protocol.CommandDetail{
			Name:        cmd.Name(),
			Description: cmd.Description(),
			Namespace:   cmd.Namespace(),
			InputSchema: protocol.SchemaInfo{
				Type:       inSchema.Type,
				Properties: convertSchemaFields(inSchema.Properties),
				Required:   inSchema.Required,
			},
			OutputSchema: protocol.SchemaInfo{
				Type:       outSchema.Type,
				Properties: convertSchemaFields(outSchema.Properties),
				Required:   outSchema.Required,
			},
			Credentials: cmd.RequiredCredentials(),
		}, nil
	})

	// execute
	h.Register(protocol.MethodExecute, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ExecuteParams](params)
		if err != nil {
			return nil, err
		}

		cmd, resolveErr := registry.Resolve(p.Command)
		if resolveErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeCommandNotFound, Message: resolveErr.Error()}
		}

		// Build input envelope from args.
		input := agshctx.NewEnvelope(p.Args, "application/json", "agent")

		bus.Publish(events.Event{
			Type:      events.EventCommandStart,
			Timestamp: time.Now(),
			Data:      map[string]any{"command": p.Command, "intent": p.Intent},
		})

		start := time.Now()
		output, execErr := cmd.Execute(gocontext.Background(), input, store)
		duration := time.Since(start)

		if execErr != nil {
			bus.Publish(events.Event{
				Type:      events.EventCommandError,
				Timestamp: time.Now(),
				Data:      map[string]any{"command": p.Command, "error": execErr.Error()},
				Duration:  duration,
			})
			return nil, &protocol.Error{Code: protocol.CodeCommandFailed, Message: execErr.Error()}
		}

		bus.Publish(events.Event{
			Type:      events.EventCommandEnd,
			Timestamp: time.Now(),
			Data:      map[string]any{"command": p.Command, "status": "ok"},
			Duration:  duration,
		})

		result := protocol.ExecuteResult{
			Payload: output.Payload,
			Meta: map[string]any{
				"content_type": output.Meta.ContentType,
				"source":       output.Meta.Source,
				"tags":         output.Meta.Tags,
			},
		}

		// Run verification if requested.
		if len(p.Verify) > 0 {
			intent := assertionDefsToIntent(p.Verify, p.Intent)
			engine := verify.NewEngine()

			bus.Publish(events.NewEvent(events.EventVerifyStart, map[string]any{
				"command":    p.Command,
				"assertions": len(p.Verify),
			}))

			vResult, _ := engine.Verify(output, intent)
			result.Verification = &protocol.VerificationInfo{
				Passed:  vResult.Passed,
				Results: convertVerifyResults(vResult.Results),
			}

			bus.Publish(events.NewEvent(events.EventVerifyResult, map[string]any{
				"command": p.Command,
				"passed":  vResult.Passed,
			}))
		}

		// Add provenance.
		for _, step := range output.Provenance {
			result.Provenance = append(result.Provenance, protocol.ProvenanceStep{
				Command:  step.Command,
				Duration: step.Duration.String(),
				Status:   step.Status,
			})
		}

		return result, nil
	})

	// pipeline
	h.Register(protocol.MethodPipeline, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.PipelineParams](params)
		if err != nil {
			return nil, err
		}

		steps := make([]agshctx.PipelineStep, len(p.Steps))
		for i, s := range p.Steps {
			steps[i] = agshctx.PipelineStep{
				Command: s.Command,
				Intent:  s.Intent,
				OnError: s.OnError,
			}
		}

		executor := &registryExecutor{registry: registry}
		publisher := &eventBusPublisher{bus: bus}

		pipeline := &agshctx.Pipeline{
			Steps:    steps,
			Context:  store,
			Executor: executor,
			Events:   publisher,
		}

		if cpMgr != nil {
			pipeline.Checkpointer = &checkpointAdapter{
				manager: cpMgr,
				store:   store,
			}
		}

		ctx := gocontext.Background()
		input := agshctx.NewEnvelope(nil, "text/plain", "agent")

		result, execErr := pipeline.Run(ctx, input)
		if execErr != nil {
			return map[string]any{
				"success": false,
				"error":   execErr.Error(),
				"steps":   len(result.Steps),
			}, nil
		}

		return map[string]any{
			"success": result.Success,
			"steps":   len(result.Steps),
			"output":  result.Output.Payload,
		}, nil
	})

	// context.get
	h.Register(protocol.MethodContextGet, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ContextGetParams](params)
		if err != nil {
			return nil, err
		}
		val, getErr := store.Get(p.Scope, p.Key)
		if getErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: getErr.Error()}
		}
		return val, nil
	})

	// context.set
	h.Register(protocol.MethodContextSet, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ContextSetParams](params)
		if err != nil {
			return nil, err
		}
		if setErr := store.Set(p.Scope, p.Key, p.Value); setErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: setErr.Error()}
		}

		bus.Publish(events.NewEvent(events.EventContextChange, map[string]any{
			"scope": p.Scope,
			"key":   p.Key,
		}))

		return "ok", nil
	})

	// checkpoint.save
	h.Register(protocol.MethodCheckpointSave, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.CheckpointParams](params)
		if err != nil {
			return nil, err
		}
		if cpMgr == nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: "checkpoint manager not available"}
		}
		snap, snapErr := verify.CaptureSnapshot(store, "")
		if snapErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: snapErr.Error()}
		}
		if saveErr := cpMgr.Save(p.Name, snap); saveErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: saveErr.Error()}
		}

		bus.Publish(events.NewEvent(events.EventCheckpointSave, map[string]any{
			"name": p.Name,
		}))

		return map[string]any{"saved": p.Name}, nil
	})

	// checkpoint.restore
	h.Register(protocol.MethodCheckpointRestore, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.CheckpointParams](params)
		if err != nil {
			return nil, err
		}
		if cpMgr == nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: "checkpoint manager not available"}
		}
		snap, restoreErr := cpMgr.Restore(p.Name)
		if restoreErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: restoreErr.Error()}
		}
		if err := verify.RestoreSnapshot(store, snap); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: err.Error()}
		}

		bus.Publish(events.NewEvent(events.EventCheckpointRestore, map[string]any{
			"name": p.Name,
		}))

		return map[string]any{"restored": p.Name}, nil
	})

	// history
	h.Register(protocol.MethodHistory, func(params json.RawMessage) (any, *protocol.Error) {
		history := bus.History(time.Time{})
		return history, nil
	})
}

// registerProjectMethods registers project.* lifecycle methods.
func registerProjectMethods(h *protocol.Handler, registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus, state *agentState, cpMgr verify.CheckpointManager) {
	// project.load
	h.Register(protocol.MethodProjectLoad, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ProjectLoadParams](params)
		if err != nil {
			return nil, err
		}

		projSpec, loadErr := spec.LoadSpec(p.Path, p.Params)
		if loadErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: loadErr.Error()}
		}

		vr := spec.ValidateSpec(projSpec)
		if !vr.Valid() {
			return nil, &protocol.Error{Code: protocol.CodeSpecInvalid, Message: vr.Error()}
		}

		state.mu.Lock()
		state.loadedSpec = &projSpec
		state.pendingPlan = nil
		state.planID = ""
		state.mu.Unlock()

		bus.Publish(events.NewEvent(events.EventSpecLoaded, map[string]any{
			"name":        projSpec.Meta.Name,
			"description": projSpec.Meta.Description,
			"goal":        projSpec.Goal,
		}))

		return map[string]any{
			"name":            projSpec.Meta.Name,
			"description":     projSpec.Meta.Description,
			"goal":            projSpec.Goal,
			"constraints":     projSpec.Constraints,
			"success_criteria": len(projSpec.SuccessCriteria),
			"params":          projSpec.Params,
		}, nil
	})

	// project.plan
	h.Register(protocol.MethodProjectPlan, func(params json.RawMessage) (any, *protocol.Error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		if state.loadedSpec == nil {
			return nil, &protocol.Error{Code: protocol.CodeNoPendingPlan, Message: "no spec loaded; call project.load first"}
		}

		lister := &registryLister{registry: registry}
		plan, planErr := spec.GeneratePlan(*state.loadedSpec, lister)
		if planErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: planErr.Error()}
		}

		state.pendingPlan = &plan
		state.planID = fmt.Sprintf("plan-%d", time.Now().UnixMilli())

		bus.Publish(events.NewEvent(events.EventPlanGenerated, map[string]any{
			"plan_id":       state.planID,
			"spec":          plan.Spec,
			"steps":         len(plan.Steps),
			"risk_summary":  plan.EstimatedRisk,
		}))

		bus.Publish(events.NewEvent(events.EventPlanApproval, map[string]any{
			"plan_id": state.planID,
			"message": "plan awaiting approval",
		}))

		// Return the plan for review.
		planSteps := make([]map[string]any, len(plan.Steps))
		for i, step := range plan.Steps {
			planSteps[i] = map[string]any{
				"command":           step.Command,
				"args":              step.Args,
				"intent":            step.Intent,
				"risk":              step.Risk,
				"checkpoint_before": step.CheckpointBefore,
				"on_error":          step.OnError,
			}
		}

		return map[string]any{
			"plan_id":          state.planID,
			"spec":             plan.Spec,
			"steps":            planSteps,
			"risk_summary":     plan.EstimatedRisk,
			"success_criteria": len(plan.SuccessCriteria),
			"status":           "awaiting_approval",
		}, nil
	})

	// project.approve
	h.Register(protocol.MethodProjectApprove, func(params json.RawMessage) (any, *protocol.Error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		if state.pendingPlan == nil {
			return nil, &protocol.Error{Code: protocol.CodeNoPendingPlan, Message: "no pending plan to approve"}
		}

		bus.Publish(events.NewEvent(events.EventPlanApproved, map[string]any{
			"plan_id": state.planID,
		}))

		// Execute the plan.
		plan := *state.pendingPlan
		state.pendingPlan = nil

		result, execErr := executeAgentPlan(plan, registry, store, bus, cpMgr)
		if execErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeCommandFailed, Message: execErr.Error()}
		}

		return result, nil
	})

	// project.reject
	h.Register(protocol.MethodProjectReject, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ProjectRejectParams](params)
		if err != nil {
			return nil, err
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		if state.pendingPlan == nil {
			return nil, &protocol.Error{Code: protocol.CodeNoPendingPlan, Message: "no pending plan to reject"}
		}

		bus.Publish(events.NewEvent(events.EventPlanRejected, map[string]any{
			"plan_id":  state.planID,
			"feedback": p.Feedback,
		}))

		state.pendingPlan = nil
		state.planID = ""

		return map[string]any{"status": "rejected", "feedback": p.Feedback}, nil
	})

	// project.run — load + plan + auto-approve + execute.
	h.Register(protocol.MethodProjectRun, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ProjectLoadParams](params)
		if err != nil {
			return nil, err
		}

		projSpec, loadErr := spec.LoadSpec(p.Path, p.Params)
		if loadErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: loadErr.Error()}
		}

		vr := spec.ValidateSpec(projSpec)
		if !vr.Valid() {
			return nil, &protocol.Error{Code: protocol.CodeSpecInvalid, Message: vr.Error()}
		}

		bus.Publish(events.NewEvent(events.EventSpecLoaded, map[string]any{
			"name": projSpec.Meta.Name,
		}))

		lister := &registryLister{registry: registry}
		plan, planErr := spec.GeneratePlan(projSpec, lister)
		if planErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: planErr.Error()}
		}

		bus.Publish(events.NewEvent(events.EventPlanGenerated, map[string]any{
			"spec":  plan.Spec,
			"steps": len(plan.Steps),
		}))

		bus.Publish(events.NewEvent(events.EventPlanApproved, map[string]any{
			"auto": true,
		}))

		result, execErr := executeAgentPlan(plan, registry, store, bus, cpMgr)
		if execErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeCommandFailed, Message: execErr.Error()}
		}

		return result, nil
	})

	// project.validate
	h.Register(protocol.MethodProjectValidate, func(params json.RawMessage) (any, *protocol.Error) {
		p, err := protocol.ParseParams[protocol.ProjectLoadParams](params)
		if err != nil {
			return nil, err
		}

		projSpec, loadErr := spec.LoadSpec(p.Path, p.Params)
		if loadErr != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: loadErr.Error()}
		}

		vr := spec.ValidateSpec(projSpec)
		errors := make([]map[string]string, len(vr.Errors))
		for i, e := range vr.Errors {
			errors[i] = map[string]string{"field": e.Field, "message": e.Message}
		}

		return map[string]any{
			"valid":  vr.Valid(),
			"errors": errors,
		}, nil
	})

	// project.init
	h.Register(protocol.MethodProjectInit, func(params json.RawMessage) (any, *protocol.Error) {
		var p struct {
			Template string `json:"template"`
			Output   string `json:"output"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: err.Error()}
		}
		if p.Output == "" {
			p.Output = "project.agsh.yaml"
		}
		// Delegate to the scaffolding function.
		if err := scaffoldFromTemplate(p.Template, p.Output); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternalError, Message: err.Error()}
		}
		return map[string]any{"created": p.Output, "template": p.Template}, nil
	})
}

// executeAgentPlan runs a plan through the pipeline and verifies success criteria.
func executeAgentPlan(plan spec.ExecutionPlan, registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus, cpMgr verify.CheckpointManager) (map[string]any, error) {
	executor := &registryExecutor{registry: registry}
	publisher := &eventBusPublisher{bus: bus}

	pipelineSteps := make([]agshctx.PipelineStep, len(plan.Steps))
	for i, step := range plan.Steps {
		pipelineSteps[i] = agshctx.PipelineStep{
			Command:          step.Command,
			Args:             step.Args,
			Intent:           step.Intent,
			OnError:          step.OnError,
			CheckpointBefore: step.CheckpointBefore,
		}
	}

	pipeline := &agshctx.Pipeline{
		Steps:    pipelineSteps,
		Context:  store,
		Executor: executor,
		Events:   publisher,
	}

	if cpMgr != nil {
		pipeline.Checkpointer = &checkpointAdapter{
			manager: cpMgr,
			store:   store,
		}
	}

	ctx := gocontext.Background()
	input := agshctx.NewEnvelope(nil, "text/plain", "agent")

	result, execErr := pipeline.Run(ctx, input)
	if execErr != nil {
		return nil, execErr
	}

	response := map[string]any{
		"success": result.Success,
		"steps":   len(result.Steps),
		"output":  result.Output.Payload,
	}

	// Verify success criteria.
	if len(plan.SuccessCriteria) > 0 {
		intent := specCriteriaToIntent(plan.SuccessCriteria)
		engine := verify.NewEngine()

		bus.Publish(events.NewEvent(events.EventVerifyStart, map[string]any{
			"type":       "success_criteria",
			"assertions": len(plan.SuccessCriteria),
		}))

		vResult, _ := engine.Verify(result.Output, intent)

		bus.Publish(events.NewEvent(events.EventVerifyResult, map[string]any{
			"passed":     vResult.Passed,
			"assertions": len(vResult.Results),
		}))

		response["verification"] = map[string]any{
			"passed":  vResult.Passed,
			"results": convertVerifyResults(vResult.Results),
		}

		if !vResult.Passed {
			return nil, fmt.Errorf("verification failed: %d/%d assertions passed",
				countPassed(vResult.Results), len(vResult.Results))
		}
	}

	return response, nil
}

// Helper functions.

func convertSchemaFields(fields map[string]platform.SchemaField) map[string]protocol.SchemaFieldInfo {
	if fields == nil {
		return nil
	}
	result := make(map[string]protocol.SchemaFieldInfo, len(fields))
	for k, v := range fields {
		result[k] = protocol.SchemaFieldInfo{Type: v.Type, Description: v.Description}
	}
	return result
}

func assertionDefsToIntent(defs []protocol.AssertionDef, intentDesc string) verify.Intent {
	assertions := make([]verify.Assertion, len(defs))
	for i, d := range defs {
		assertions[i] = verify.Assertion{
			Type:     d.Type,
			Target:   d.Target,
			Expected: d.Expected,
		}
	}
	return verify.Intent{
		Description: intentDesc,
		Assertions:  assertions,
	}
}

func convertVerifyResults(results []verify.AssertionResult) []protocol.AssertionOutput {
	out := make([]protocol.AssertionOutput, len(results))
	for i, r := range results {
		out[i] = protocol.AssertionOutput{
			Type:    r.Assertion.Type,
			Passed:  r.Passed,
			Actual:  r.Actual,
			Message: r.Message,
		}
	}
	return out
}
