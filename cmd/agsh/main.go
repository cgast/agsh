package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cgast/agsh/internal/config"
	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/platform/fs"
	ghplatform "github.com/cgast/agsh/pkg/platform/github"
	httpplatform "github.com/cgast/agsh/pkg/platform/http"
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

	// Load configuration.
	cfg, err := config.LoadConfig(configPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading config: %v\n", err)
	}
	platCfg, err := config.LoadPlatformConfig(platformConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading platform config: %v\n", err)
	}

	// Override mode from config if not set via flag/env.
	if mode == "" && cfg.Mode != "" {
		mode = cfg.Mode
	}

	// Initialize core components.
	bus := events.NewMemoryBus()
	registry := platform.NewRegistry()
	registerCommands(registry, platCfg)

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

func registerCommands(registry *platform.Registry, platCfg config.PlatformConfig) {
	// Built-in filesystem commands.
	registry.Register(&fs.ListCommand{})
	registry.Register(&fs.ReadCommand{})
	registry.Register(&fs.WriteCommand{})

	// GitHub commands (only if token is configured).
	if platCfg.GitHub.Token != "" {
		ghClient, err := ghplatform.NewClient(platCfg.GitHub.Token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: github client init: %v\n", err)
		} else {
			registry.Register(ghplatform.NewRepoInfoCommand(ghClient))
			registry.Register(ghplatform.NewPRListCommand(ghClient))
			registry.Register(ghplatform.NewIssueCreateCommand(ghClient))
		}
	}

	// HTTP commands (with domain allowlisting).
	registry.Register(httpplatform.NewGetCommand(platCfg.HTTP.AllowedDomains))
	registry.Register(httpplatform.NewPostCommand(platCfg.HTTP.AllowedDomains))
}

func configPath() string {
	return filepath.Join(".agsh", "config.yaml")
}

func platformConfigPath() string {
	return filepath.Join(".agsh", "platforms.yaml")
}

func contextStorePath() string {
	// Use project-local .agsh directory if it exists, otherwise temp.
	if _, err := os.Stat(".agsh"); err == nil {
		return filepath.Join(".agsh", "context.db")
	}
	return filepath.Join(os.TempDir(), "agsh-context.db")
}
