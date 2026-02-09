package main

import (
	"fmt"
	"os"
	"path/filepath"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/platform/fs"
)

func main() {
	// Check for demo mode.
	if len(os.Args) >= 2 && os.Args[1] == "demo" {
		if err := handleDemo(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	mode := detectMode()

	// Initialize core components.
	bus := events.NewMemoryBus()
	registry := platform.NewRegistry()
	registerCommands(registry)

	// Initialize context store.
	dbPath := contextStorePath()
	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to open context store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	switch mode {
	case "interactive":
		runInteractiveREPL(registry, store, bus)
	case "agent":
		fmt.Fprintln(os.Stderr, "agent mode not yet implemented (Phase 4)")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(1)
	}
}

func handleDemo() error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agsh demo <number> [workspace-dir] [output-path]")
		fmt.Println("  agsh demo 01 ./examples/demo/01-basic-pipeline/workspace ./output.md")
		return nil
	}
	switch os.Args[2] {
	case "01":
		workspaceDir := "./examples/demo/01-basic-pipeline/workspace"
		outputPath := "./examples/demo/01-basic-pipeline/output.md"
		if len(os.Args) >= 4 {
			workspaceDir = os.Args[3]
		}
		if len(os.Args) >= 5 {
			outputPath = os.Args[4]
		}
		return runDemo01(workspaceDir, outputPath)
	default:
		return fmt.Errorf("unknown demo: %s", os.Args[2])
	}
}

func detectMode() string {
	// Check for explicit --mode flag.
	for i, arg := range os.Args[1:] {
		if arg == "--mode=agent" {
			return "agent"
		}
		if arg == "--mode" && i+2 < len(os.Args) {
			return os.Args[i+2]
		}
	}

	// Check environment variable.
	if m := os.Getenv("AGSH_MODE"); m != "" {
		return m
	}

	// Default based on terminal detection.
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "interactive"
	}
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		return "interactive"
	}

	return "interactive"
}

func registerCommands(registry *platform.Registry) {
	registry.Register(&fs.ListCommand{})
	registry.Register(&fs.ReadCommand{})
	registry.Register(&fs.WriteCommand{})
}

func contextStorePath() string {
	// Use project-local .agsh directory if it exists, otherwise temp.
	if _, err := os.Stat(".agsh"); err == nil {
		return filepath.Join(".agsh", "context.db")
	}
	return filepath.Join(os.TempDir(), "agsh-context.db")
}
