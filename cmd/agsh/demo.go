package main

import (
	"fmt"
	gocontext "context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/internal/config"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/platform/fs"
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
