package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cgast/agsh/internal/config"
	"github.com/cgast/agsh/internal/inspector"
	"github.com/cgast/agsh/internal/sandbox"
	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
	"github.com/cgast/agsh/pkg/platform/fs"
	ghplatform "github.com/cgast/agsh/pkg/platform/github"
	httpplatform "github.com/cgast/agsh/pkg/platform/http"
	"github.com/cgast/agsh/pkg/verify"
)

func main() {
	// Check for subcommands that don't need full initialization.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "demo":
			if err := handleDemo(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "init":
			if err := handleInit(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "validate":
			if err := handleValidate(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
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

	// Create sandbox from config for filesystem enforcement.
	var sb *sandbox.Sandbox
	if len(cfg.Sandbox.AllowedPaths) > 0 || len(cfg.Sandbox.DeniedPaths) > 0 || cfg.Sandbox.MaxFileSize != "" {
		var err error
		sb, err = sandbox.New(sandbox.Config{
			AllowedPaths: cfg.Sandbox.AllowedPaths,
			DeniedPaths:  cfg.Sandbox.DeniedPaths,
			MaxFileSize:  cfg.Sandbox.MaxFileSize,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: sandbox init: %v\n", err)
		}
	}
	registerCommandsSandboxed(registry, platCfg, sb)

	// Initialize context store.
	dbPath := contextStorePath()
	store, err := agshctx.NewBoltStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to open context store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Start inspector if enabled via flag or config.
	inspectorPort := detectInspectorPort(cfg)
	if inspectorPort > 0 {
		cpDir := filepath.Join(os.TempDir(), "agsh-checkpoints")
		cpMgr, _ := verify.NewFileCheckpointManager(cpDir)
		srv := inspector.New(bus, store, registry, cpMgr)
		srv.StartAsync(inspectorPort)
		fmt.Fprintf(os.Stderr, "Inspector running at http://localhost:%d\n", inspectorPort)
	}

	// Handle subcommands that need full initialization.
	if len(os.Args) >= 2 && os.Args[1] == "run" {
		if err := handleRun(registry, store, bus); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch mode {
	case "interactive":
		runInteractiveREPL(registry, store, bus)
	case "agent":
		runAgentMode(registry, store, bus)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(1)
	}
}

func handleDemo() error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agsh demo <number> [args...]")
		fmt.Println("  agsh demo 01 [workspace-dir] [output-path]")
		fmt.Println("  agsh demo 02 [spec-path]")
		fmt.Println("  agsh demo 03 [input-csv]")
		fmt.Println("  agsh demo 04 [spec-path]")
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
	case "02":
		specPath := "./examples/demo/02-github-report/project.agsh.yaml"
		if len(os.Args) >= 4 {
			specPath = os.Args[3]
		}
		return runDemo02(specPath)
	case "03":
		inputCSV := "./examples/demo/03-verified-transform/workspace/team.csv"
		if len(os.Args) >= 4 {
			inputCSV = os.Args[3]
		}
		return runDemo03(inputCSV)
	case "04":
		specPath := "./examples/demo/04-agent-autonomy/local-spec.agsh.yaml"
		if len(os.Args) >= 4 {
			specPath = os.Args[3]
		}
		return runDemo04(specPath)
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
	registerCommandsSandboxed(registry, platCfg, nil)
}

func registerCommandsSandboxed(registry *platform.Registry, platCfg config.PlatformConfig, sb *sandbox.Sandbox) {
	// Built-in filesystem commands with optional sandbox enforcement.
	registry.Register(&fs.ListCommand{Sandbox: sb})
	registry.Register(&fs.ReadCommand{Sandbox: sb})
	registry.Register(&fs.WriteCommand{Sandbox: sb})

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

// detectInspectorPort parses --inspector and --inspector-port flags.
// Returns 0 if the inspector is disabled, or the port number to use.
func detectInspectorPort(cfg config.Config) int {
	const defaultPort = 4200

	// Check for --no-inspector flag.
	for _, arg := range os.Args[1:] {
		if arg == "--no-inspector" {
			return 0
		}
	}

	// Check for --inspector or --inspector-port flag.
	for _, arg := range os.Args[1:] {
		if arg == "--inspector" {
			return defaultPort
		}
		if strings.HasPrefix(arg, "--inspector-port=") {
			portStr := strings.TrimPrefix(arg, "--inspector-port=")
			if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
				return port
			}
		}
	}

	// Check AGSH_INSPECTOR env var.
	if envVal := os.Getenv("AGSH_INSPECTOR"); envVal != "" {
		if port, err := strconv.Atoi(envVal); err == nil && port > 0 {
			return port
		}
		// "true" or any non-empty value enables on default port.
		return defaultPort
	}

	// Check config file.
	if cfg.Inspector.Enabled {
		if cfg.Inspector.Port > 0 {
			return cfg.Inspector.Port
		}
		return defaultPort
	}

	return 0
}
