package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	gocontext "context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/internal/config"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/platform/fs"
	"github.com/cgast/agsh/pkg/protocol"
	"github.com/cgast/agsh/pkg/verify"
)

// runDemo01 executes the heading-counter demo pipeline end-to-end.
// This hardcodes the pipeline since spec loading isn't implemented yet.
func runDemo01(workspaceDir, outputPath string) error {
	bus := events.NewMemoryBus()
	registry := platform.NewRegistry()
	registerCommands(registry, config.PlatformConfig{})

	dbPath := filepath.Join(os.TempDir(), "agsh-demo01.db")
	defer os.Remove(dbPath)

	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer store.Close()

	ctx := gocontext.Background()
	publisher := &eventBusPublisher{bus: bus}

	// Subscribe to events for observability.
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)
	go func() {
		for ev := range ch {
			fmt.Fprintf(os.Stderr, "[event] %s: %v\n", ev.Type, ev.Data)
		}
	}()

	// Step 1: List markdown files in workspace.
	fmt.Fprintf(os.Stderr, "=== Demo 01: Heading Counter ===\n")
	fmt.Fprintf(os.Stderr, "Workspace: %s\n", workspaceDir)

	listCmd := &fs.ListCommand{}
	listInput := agshctx.NewEnvelope(workspaceDir, "text/plain", "demo")

	publisher.PublishPipelineEvent("pipeline.start", map[string]any{
		"demo": "01-heading-counter",
		"step_count": 3,
	}, 0, 0)

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "fs:list",
	}, 0, 0)
	listOutput, err := listCmd.Execute(ctx, listInput, store)
	if err != nil {
		return fmt.Errorf("fs:list: %w", err)
	}
	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "fs:list",
		"status": "ok",
	}, 0, 0)

	files, ok := listOutput.Payload.([]fs.FileEntry)
	if !ok {
		return fmt.Errorf("unexpected payload type from fs:list: %T", listOutput.Payload)
	}

	// Filter to .md files and sort alphabetically.
	var mdFiles []fs.FileEntry
	for _, f := range files {
		if !f.IsDir && strings.HasSuffix(f.Name, ".md") {
			mdFiles = append(mdFiles, f)
		}
	}
	sort.Slice(mdFiles, func(i, j int) bool {
		return mdFiles[i].Name < mdFiles[j].Name
	})

	fmt.Fprintf(os.Stderr, "Found %d markdown files\n", len(mdFiles))

	// Step 2: Read each file and count headings.
	type fileCount struct {
		Name  string
		Count int
	}

	readCmd := &fs.ReadCommand{}
	var counts []fileCount
	totalHeadings := 0

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "fs:read",
		"intent": "Read each markdown file and count headings",
	}, 1, 0)

	for _, f := range mdFiles {
		readInput := agshctx.NewEnvelope(f.Path, "text/plain", "demo")
		readOutput, err := readCmd.Execute(ctx, readInput, store)
		if err != nil {
			return fmt.Errorf("fs:read %s: %w", f.Name, err)
		}

		content := readOutput.PayloadString()
		headingCount := 0
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				headingCount++
			}
		}

		counts = append(counts, fileCount{Name: f.Name, Count: headingCount})
		totalHeadings += headingCount

		// Store per-file count in context.
		store.Set(agshctx.ScopeSession, "heading_count_"+f.Name, headingCount)
		fmt.Fprintf(os.Stderr, "  %s: %d headings\n", f.Name, headingCount)
	}

	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "fs:read",
		"status": "ok",
		"files_read": len(mdFiles),
	}, 1, 0)

	store.Set(agshctx.ScopeSession, "total_headings", totalHeadings)
	store.Set(agshctx.ScopeSession, "file_count", len(mdFiles))

	// Step 3: Generate and write summary.
	var sb strings.Builder
	sb.WriteString("# Heading Summary\n\n")
	sb.WriteString("| File | Headings |\n")
	sb.WriteString("|------|----------|\n")
	for _, fc := range counts {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", fc.Name, fc.Count))
	}
	sb.WriteString(fmt.Sprintf("\n**Total: %d headings across %d files**\n", totalHeadings, len(mdFiles)))

	summary := sb.String()

	writeCmd := &fs.WriteCommand{}
	writeInput := agshctx.NewEnvelope(map[string]any{
		"path":    outputPath,
		"content": summary,
	}, "application/json", "demo")

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "fs:write",
	}, 2, 0)
	_, err = writeCmd.Execute(ctx, writeInput, store)
	if err != nil {
		return fmt.Errorf("fs:write: %w", err)
	}
	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "fs:write",
		"status": "ok",
	}, 2, 0)

	publisher.PublishPipelineEvent("pipeline.end", map[string]any{
		"success": true,
	}, 2, 0)

	// Verify: output is not empty.
	if summary == "" {
		return fmt.Errorf("verification failed: summary is empty")
	}

	fmt.Fprintf(os.Stderr, "\n=== Output ===\n")
	fmt.Print(summary)
	fmt.Fprintf(os.Stderr, "\nWritten to: %s\n", outputPath)
	fmt.Fprintf(os.Stderr, "=== Demo 01 Complete ===\n")

	return nil
}

// runDemo03 executes the verified-transform demo: CSV → markdown table with verification.
func runDemo03(inputCSV string) error {
	bus := events.NewMemoryBus()
	registry := platform.NewRegistry()
	registerCommands(registry, config.PlatformConfig{})

	dbPath := filepath.Join(os.TempDir(), "agsh-demo03.db")
	defer os.Remove(dbPath)

	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer store.Close()

	ctx := gocontext.Background()
	publisher := &eventBusPublisher{bus: bus}

	// Subscribe to events for observability.
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)
	go func() {
		for ev := range ch {
			fmt.Fprintf(os.Stderr, "[event] %s: %v\n", ev.Type, ev.Data)
		}
	}()

	fmt.Fprintf(os.Stderr, "=== Demo 03: Verified File Transform ===\n")
	fmt.Fprintf(os.Stderr, "Input: %s\n", inputCSV)

	// Set up checkpoint manager.
	cpDir := filepath.Join(os.TempDir(), "agsh-demo03-checkpoints")
	cpMgr, err := verify.NewFileCheckpointManager(cpDir)
	if err != nil {
		return fmt.Errorf("create checkpoint manager: %w", err)
	}

	publisher.PublishPipelineEvent("pipeline.start", map[string]any{
		"demo":       "03-verified-transform",
		"step_count": 3,
	}, 0, 0)

	// Step 1: Read the CSV file.
	fmt.Fprintf(os.Stderr, "\nStep 1: Reading CSV file...\n")
	readCmd := &fs.ReadCommand{}
	readInput := agshctx.NewEnvelope(inputCSV, "text/plain", "demo")

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "fs:read",
		"intent":  "Read CSV input file",
	}, 0, 0)
	readOutput, err := readCmd.Execute(ctx, readInput, store)
	if err != nil {
		return fmt.Errorf("fs:read %s: %w", inputCSV, err)
	}
	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "fs:read",
		"status":  "ok",
	}, 0, 0)

	csvContent := readOutput.PayloadString()
	fmt.Fprintf(os.Stderr, "  Read %d bytes\n", len(csvContent))

	// Step 2: Transform CSV to markdown table.
	fmt.Fprintf(os.Stderr, "\nStep 2: Transforming CSV to markdown table...\n")

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "transform",
		"intent":  "Convert CSV to sorted markdown table with totals",
	}, 1, 0)

	tableOutput, err := csvToMarkdownTable(csvContent)
	if err != nil {
		publisher.PublishPipelineEvent("command.error", map[string]any{
			"command": "transform",
			"error":   err.Error(),
		}, 1, 0)
		return fmt.Errorf("transform: %w", err)
	}

	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "transform",
		"status":  "ok",
	}, 1, 0)

	fmt.Fprintf(os.Stderr, "  Generated %d lines of markdown\n", len(strings.Split(tableOutput, "\n")))

	// Step 3: Verify the output before writing.
	fmt.Fprintf(os.Stderr, "\nStep 3: Verifying output...\n")

	// Save a checkpoint before writing.
	snap, err := verify.CaptureSnapshot(store, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: checkpoint capture failed: %v\n", err)
	} else {
		if err := cpMgr.Save("pre-write", snap); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: checkpoint save failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "  Checkpoint saved: pre-write\n")
		}
	}

	// Build the verification intent from spec's success_criteria.
	intent := verify.Intent{
		Description: "Verify CSV-to-markdown transform output",
		Assertions: []verify.Assertion{
			{Type: "not_empty", Target: "output", Message: "Output must not be empty"},
			{Type: "contains", Target: "output", Expected: "|", Message: "Output must contain markdown table pipes"},
			{Type: "contains", Target: "output", Expected: "---", Message: "Output must contain markdown table separator"},
			{Type: "count_gte", Target: "output.lines", Expected: 10, Message: "Output must have at least 10 lines (title + blank + header + separator + 5 data rows + summary)"},
			{Type: "matches_regex", Target: "output", Expected: `\| Name\s*\|`, Message: "Table must start with a Name column"},
			{Type: "not_contains", Target: "output", Expected: ",,,", Message: "Output must not contain raw CSV artifacts"},
			{Type: "not_contains", Target: "output", Expected: "|  |", Message: "Data cells must not be empty"},
			{Type: "llm_judge", Target: "output", Expected: "A well-formatted markdown table with all 5 team members, sorted alphabetically, with a totals row", Message: "Overall output quality check"},
		},
	}

	outputEnvelope := agshctx.NewEnvelope(tableOutput, "text/markdown", "transform")
	engine := verify.NewEngine()
	vResult, err := engine.Verify(outputEnvelope, intent)
	if err != nil {
		return fmt.Errorf("verification error: %w", err)
	}

	publisher.PublishPipelineEvent("verify.result", map[string]any{
		"passed":     vResult.Passed,
		"assertions": len(vResult.Results),
	}, 2, 0)

	fmt.Fprintf(os.Stderr, "\n=== Verification Results ===\n")
	for _, ar := range vResult.Results {
		status := "PASS"
		if !ar.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "  [%s] %s: %s\n", status, ar.Assertion.Type, ar.Message)
	}

	if !vResult.Passed {
		fmt.Fprintf(os.Stderr, "\nVerification FAILED. Rolling back to checkpoint.\n")

		// Rollback to pre-write checkpoint.
		restored, restoreErr := cpMgr.Restore("pre-write")
		if restoreErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: rollback failed: %v\n", restoreErr)
		} else {
			if err := verify.RestoreSnapshot(store, restored); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: restore failed: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "  Rolled back to pre-write checkpoint.\n")
			}
		}

		publisher.PublishPipelineEvent("pipeline.end", map[string]any{
			"success":        false,
			"verify_failure": true,
		}, 2, 0)

		passed := 0
		for _, ar := range vResult.Results {
			if ar.Passed {
				passed++
			}
		}
		return fmt.Errorf("verification failed: %d/%d assertions passed", passed, len(vResult.Results))
	}

	fmt.Fprintf(os.Stderr, "\nAll %d assertions passed.\n", len(vResult.Results))

	// Write the verified output.
	outputPath := "./examples/demo/03-verified-transform/team-table.md"
	writeCmd := &fs.WriteCommand{}
	writeInput := agshctx.NewEnvelope(map[string]any{
		"path":    outputPath,
		"content": tableOutput,
	}, "application/json", "demo")

	publisher.PublishPipelineEvent("command.start", map[string]any{
		"command": "fs:write",
	}, 2, 0)
	_, err = writeCmd.Execute(ctx, writeInput, store)
	if err != nil {
		return fmt.Errorf("fs:write: %w", err)
	}
	publisher.PublishPipelineEvent("command.end", map[string]any{
		"command": "fs:write",
		"status":  "ok",
	}, 2, 0)

	publisher.PublishPipelineEvent("pipeline.end", map[string]any{
		"success": true,
	}, 2, 0)

	fmt.Fprintf(os.Stderr, "\n=== Output ===\n")
	fmt.Print(tableOutput)
	fmt.Fprintf(os.Stderr, "\nWritten to: %s\n", outputPath)
	fmt.Fprintf(os.Stderr, "=== Demo 03 Complete ===\n")

	return nil
}

// runDemo04 exercises the full agent autonomy loop via JSON-RPC.
// It simulates what an LLM orchestrator would do: discover commands, load a spec,
// generate a plan, approve it, and execute — all through the protocol handler.
func runDemo04(specPath string) error {
	bus := events.NewMemoryBus()
	registry := platform.NewRegistry()
	registerCommands(registry, config.PlatformConfig{})

	dbPath := filepath.Join(os.TempDir(), "agsh-demo04.db")
	defer os.Remove(dbPath)

	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer store.Close()

	// Subscribe to events for observability.
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)
	go func() {
		for ev := range ch {
			fmt.Fprintf(os.Stderr, "  [event] %s: %v\n", ev.Type, ev.Data)
		}
	}()

	fmt.Fprintf(os.Stderr, "=== Demo 04: Agent Autonomy ===\n")
	fmt.Fprintf(os.Stderr, "Spec: %s\n", specPath)
	fmt.Fprintf(os.Stderr, "Simulating LLM orchestrator via JSON-RPC protocol handler...\n\n")

	// Set up the protocol handler exactly as agent mode does.
	handler := protocol.NewHandler()
	state := &agentState{}

	cpDir := filepath.Join(os.TempDir(), "agsh-demo04-checkpoints")
	cpMgr, _ := verify.NewFileCheckpointManager(cpDir)

	registerCoreMethods(handler, registry, store, bus, cpMgr)
	registerProjectMethods(handler, registry, store, bus, state, cpMgr)

	// Helper to send a JSON-RPC request and display the result.
	reqID := 0
	send := func(method string, params any) (json.RawMessage, error) {
		reqID++
		var rawParams json.RawMessage
		if params != nil {
			b, err := json.Marshal(params)
			if err != nil {
				return nil, fmt.Errorf("marshal params: %w", err)
			}
			rawParams = b
		}

		req := protocol.Request{
			JSONRPC: "2.0",
			ID:      reqID,
			Method:  method,
			Params:  rawParams,
		}

		fmt.Fprintf(os.Stderr, "Agent → %s", method)
		if rawParams != nil {
			compact := string(rawParams)
			if len(compact) > 80 {
				compact = compact[:77] + "..."
			}
			fmt.Fprintf(os.Stderr, " %s", compact)
		}
		fmt.Fprintf(os.Stderr, "\n")

		resp := handler.Handle(req)

		if resp.Error != nil {
			fmt.Fprintf(os.Stderr, "  ← ERROR [%d]: %s\n\n", resp.Error.Code, resp.Error.Message)
			return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
		}

		resultJSON, _ := json.Marshal(resp.Result)
		summary := string(resultJSON)
		if len(summary) > 120 {
			summary = summary[:117] + "..."
		}
		fmt.Fprintf(os.Stderr, "  ← %s\n\n", summary)

		return resultJSON, nil
	}

	// Step 1: Discover available commands.
	fmt.Fprintf(os.Stderr, "--- Phase 1: Command Discovery ---\n")
	result, err := send(protocol.MethodCommandsList, nil)
	if err != nil {
		return err
	}

	var cmds []protocol.CommandInfo
	json.Unmarshal(result, &cmds)
	fmt.Fprintf(os.Stderr, "  Agent observes %d available commands\n\n", len(cmds))

	// Step 2: Load the project spec.
	fmt.Fprintf(os.Stderr, "--- Phase 2: Load Project Spec ---\n")
	_, err = send(protocol.MethodProjectLoad, map[string]string{"path": specPath})
	if err != nil {
		return err
	}

	// Step 3: Generate a plan.
	fmt.Fprintf(os.Stderr, "--- Phase 3: Generate Plan ---\n")
	planResult, err := send(protocol.MethodProjectPlan, nil)
	if err != nil {
		return err
	}

	var planInfo map[string]any
	json.Unmarshal(planResult, &planInfo)
	fmt.Fprintf(os.Stderr, "  Plan has %v steps, status: %v\n\n", planInfo["steps"], planInfo["status"])

	// Step 4: Save a checkpoint before execution.
	fmt.Fprintf(os.Stderr, "--- Phase 4: Save Pre-execution Checkpoint ---\n")
	_, err = send(protocol.MethodCheckpointSave, map[string]string{"name": "pre-execution"})
	if err != nil {
		return err
	}

	// Step 5: Agent decides to execute steps individually (like a real LLM would).
	// Instead of blindly approving, the agent rejects the auto-plan and executes
	// with its own understanding of what args to provide.
	fmt.Fprintf(os.Stderr, "--- Phase 5: Reject Auto-Plan, Execute Manually ---\n")
	_, err = send(protocol.MethodProjectReject, map[string]string{
		"feedback": "Auto-plan lacks proper args. Agent will execute steps manually.",
	})
	if err != nil {
		return err
	}

	workspaceDir := filepath.Dir(specPath)
	if workspaceDir == "." {
		workspaceDir = "./examples/demo/04-agent-autonomy"
	}
	wsDir := filepath.Join(workspaceDir, "workspace")

	// Step 5a: List workspace files.
	fmt.Fprintf(os.Stderr, "--- Phase 5a: List Workspace Files ---\n")
	_, err = send(protocol.MethodExecute, map[string]any{
		"command": "fs:list",
		"args":    map[string]any{"path": wsDir},
		"intent":  "Discover workspace files for analysis",
	})
	if err != nil {
		return err
	}

	// Step 5b: Read each workspace file.
	fmt.Fprintf(os.Stderr, "--- Phase 5b: Read Workspace Files ---\n")
	for _, name := range []string{"readme.md", "metrics.txt"} {
		fpath := filepath.Join(wsDir, name)
		result, err := send(protocol.MethodExecute, map[string]any{
			"command": "fs:read",
			"args":    map[string]any{"path": fpath},
			"intent":  fmt.Sprintf("Read %s for analysis", name),
		})
		if err != nil {
			return err
		}

		// Store in context (simulating what an LLM would do with the data).
		var readResult map[string]any
		json.Unmarshal(result, &readResult)
		send(protocol.MethodContextSet, map[string]any{
			"scope": "session",
			"key":   "file_" + strings.ReplaceAll(name, ".", "_"),
			"value": readResult,
		})
	}

	// Step 6: Check execution history so far.
	fmt.Fprintf(os.Stderr, "--- Phase 6: Review Execution History ---\n")
	_, err = send(protocol.MethodHistory, nil)
	if err != nil {
		return err
	}

	// Step 7: Generate the health report from gathered data.
	fmt.Fprintf(os.Stderr, "--- Phase 7: Generate & Write Health Report ---\n")

	report := generateLocalHealthReport(workspaceDir, store)

	// Save checkpoint before write.
	_, err = send(protocol.MethodCheckpointSave, map[string]string{"name": "pre-write"})
	if err != nil {
		return err
	}

	// Write the report via execute with inline verification.
	reportPath := filepath.Join(workspaceDir, "reports", "health-report.md")
	_, err = send(protocol.MethodExecute, map[string]any{
		"command": "fs:write",
		"args": map[string]any{
			"path":    reportPath,
			"content": report,
		},
		"intent": "Write the health report",
		"verify": []map[string]any{
			{"type": "not_empty", "target": "output"},
			{"type": "contains", "target": "output", "expected": "Health Score"},
			{"type": "contains", "target": "output", "expected": "Recommendation"},
		},
	})
	if err != nil {
		return err
	}

	// Print the report.
	fmt.Fprintf(os.Stderr, "\n=== Health Report ===\n")
	fmt.Print(report)
	fmt.Fprintf(os.Stderr, "\nWritten to: %s\n", reportPath)
	fmt.Fprintf(os.Stderr, "=== Demo 04 Complete ===\n")

	return nil
}

// generateLocalHealthReport builds a health report from workspace data.
func generateLocalHealthReport(workspaceDir string, store agshctx.ContextStore) string {
	// Read metrics from the workspace if available.
	metricsPath := filepath.Join(workspaceDir, "workspace", "metrics.txt")
	metrics := make(map[string]string)
	if data, err := os.ReadFile(metricsPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				metrics[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	// Count workspace files.
	readmePath := filepath.Join(workspaceDir, "workspace", "readme.md")
	readmeContent := ""
	if data, err := os.ReadFile(readmePath); err == nil {
		readmeContent = string(data)
	}

	// Compute health score from metrics.
	score := 85 // default
	commits, _ := strconv.Atoi(metrics["commits_last_30d"])
	openIssues, _ := strconv.Atoi(metrics["open_issues"])
	contributors, _ := strconv.Atoi(metrics["contributors"])
	lastCommitDays, _ := strconv.Atoi(metrics["last_commit_days_ago"])
	stars, _ := strconv.Atoi(metrics["stars"])

	if commits > 30 {
		score += 5
	}
	if openIssues > 20 {
		score -= 10
	}
	if contributors < 2 {
		score -= 15
	}
	if lastCommitDays > 7 {
		score -= 10
	}
	if score > 100 {
		score = 100
	}

	rating := "Healthy"
	if score < 70 {
		rating = "Warning"
	}
	if score < 50 {
		rating = "Critical"
	}

	var sb strings.Builder
	sb.WriteString("# Repository Health Report\n\n")
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("**Health Score: %d/100** (%s)\n\n", score, rating))
	sb.WriteString("## Metrics\n\n")
	sb.WriteString("| Metric | Value | Status |\n")
	sb.WriteString("|--------|-------|--------|\n")
	sb.WriteString(fmt.Sprintf("| Commits (30d) | %d | %s |\n", commits, statusIndicator(commits > 10)))
	sb.WriteString(fmt.Sprintf("| Open Issues | %d | %s |\n", openIssues, statusIndicator(openIssues < 20)))
	sb.WriteString(fmt.Sprintf("| Contributors | %d | %s |\n", contributors, statusIndicator(contributors >= 2)))
	sb.WriteString(fmt.Sprintf("| Days Since Last Commit | %d | %s |\n", lastCommitDays, statusIndicator(lastCommitDays < 7)))
	sb.WriteString(fmt.Sprintf("| Stars | %d | %s |\n", stars, statusIndicator(stars > 10)))
	if avgPRAge, ok := metrics["avg_pr_age_days"]; ok {
		sb.WriteString(fmt.Sprintf("| Avg PR Age (days) | %s | %s |\n", avgPRAge, statusIndicator(true)))
	}
	sb.WriteString("\n")

	if readmeContent != "" {
		headingCount := 0
		for _, line := range strings.Split(readmeContent, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				headingCount++
			}
		}
		sb.WriteString(fmt.Sprintf("## Documentation\n\nREADME has %d sections.\n\n", headingCount))
	}

	sb.WriteString("## Recommendations\n\n")
	if commits < 10 {
		sb.WriteString("- **Increase commit frequency**: Only %d commits in the last 30 days. Consider smaller, more frequent commits.\n")
	}
	if openIssues > 15 {
		sb.WriteString("- **Triage open issues**: There are many open issues. Prioritize and close stale ones.\n")
	}
	if contributors < 3 {
		sb.WriteString("- **Expand contributor base**: Only a few active contributors. Consider onboarding new team members.\n")
	}
	if lastCommitDays > 3 {
		sb.WriteString(fmt.Sprintf("- **Resume active development**: Last commit was %d days ago.\n", lastCommitDays))
	}
	sb.WriteString("- **Maintain test coverage**: Ensure all new code is tested.\n")
	sb.WriteString("- **Review and merge open PRs**: Keep PR age low for faster iteration.\n")

	return sb.String()
}

func statusIndicator(healthy bool) string {
	if healthy {
		return "Healthy"
	}
	return "Warning"
}

// csvToMarkdownTable converts CSV content to a markdown table.
// It sorts rows alphabetically by the first column and adds a totals row.
func csvToMarkdownTable(csvContent string) (string, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(csvContent)))
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) < 2 {
		return "", fmt.Errorf("CSV must have a header row and at least one data row")
	}

	header := records[0]
	rows := records[1:]

	// Sort rows alphabetically by the first column (Name).
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	// Determine which columns are numeric (for right-alignment and totals).
	numCols := len(header)
	isNumeric := make([]bool, numCols)
	totals := make([]int, numCols)

	for col := 0; col < numCols; col++ {
		allNumeric := true
		for _, row := range rows {
			if col < len(row) {
				if n, err := strconv.Atoi(strings.TrimSpace(row[col])); err != nil {
					allNumeric = false
				} else {
					totals[col] += n
				}
			}
		}
		isNumeric[col] = allNumeric
	}

	// Format header names: replace underscores, capitalize.
	displayHeaders := make([]string, numCols)
	for i, h := range header {
		h = strings.ReplaceAll(h, "_", " ")
		// Simple title case for header display.
		words := strings.Fields(h)
		for j, w := range words {
			if len(w) > 0 {
				words[j] = strings.ToUpper(w[:1]) + w[1:]
			}
		}
		displayHeaders[i] = strings.Join(words, " ")
		// Special case: "Experience Years" -> "Experience (Years)"
		displayHeaders[i] = strings.ReplaceAll(displayHeaders[i], "Experience Years", "Experience (Years)")
		displayHeaders[i] = strings.ReplaceAll(displayHeaders[i], "Projects Completed", "Projects Completed")
	}

	var sb strings.Builder

	// Title.
	sb.WriteString("# Team Overview\n\n")

	// Header row.
	sb.WriteString("|")
	for _, h := range displayHeaders {
		sb.WriteString(fmt.Sprintf(" %s |", h))
	}
	sb.WriteString("\n")

	// Separator row with alignment.
	sb.WriteString("|")
	for i := range displayHeaders {
		if isNumeric[i] {
			sb.WriteString("-------------------:|")
		} else {
			sb.WriteString("------|")
		}
	}
	sb.WriteString("\n")

	// Data rows.
	for _, row := range rows {
		sb.WriteString("|")
		for i := 0; i < numCols; i++ {
			val := ""
			if i < len(row) {
				val = strings.TrimSpace(row[i])
			}
			sb.WriteString(fmt.Sprintf(" %s |", val))
		}
		sb.WriteString("\n")
	}

	// Summary/totals row.
	sb.WriteString("|")
	for i := 0; i < numCols; i++ {
		if i == 0 {
			sb.WriteString(" **Total** |")
		} else if isNumeric[i] {
			sb.WriteString(fmt.Sprintf(" **%d** |", totals[i]))
		} else {
			sb.WriteString(" |")
		}
	}
	sb.WriteString("\n")

	return sb.String(), nil
}
