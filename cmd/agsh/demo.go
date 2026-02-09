package main

import (
	"encoding/csv"
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
