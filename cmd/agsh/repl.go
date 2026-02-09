package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	gocontext "context"
	"os"
	"strings"
	"time"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/events"
	"github.com/cgast/agsh/pkg/platform"
)

// registryExecutor adapts a platform.Registry into a context.CommandExecutor.
type registryExecutor struct {
	registry *platform.Registry
}

func (e *registryExecutor) Execute(ctx gocontext.Context, name string, input agshctx.Envelope, store agshctx.ContextStore) (agshctx.Envelope, error) {
	cmd, err := e.registry.Resolve(name)
	if err != nil {
		return agshctx.Envelope{}, err
	}
	return cmd.Execute(ctx, input, store)
}

// eventBusPublisher adapts events.EventBus into a context.EventPublisher.
type eventBusPublisher struct {
	bus events.EventBus
}

func (p *eventBusPublisher) PublishPipelineEvent(eventType string, data any, stepIndex int, duration time.Duration) {
	p.bus.Publish(events.Event{
		Type:      events.EventType(eventType),
		Timestamp: time.Now(),
		Data:      data,
		StepIndex: stepIndex,
		Duration:  duration,
	})
}

func runInteractiveREPL(registry *platform.Registry, store agshctx.ContextStore, bus *events.MemoryBus) {
	fmt.Println("agsh v0.1.0 — Agent Shell")
	fmt.Println("Type 'help' for available commands, 'exit' to quit.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	executor := &registryExecutor{registry: registry}
	publisher := &eventBusPublisher{bus: bus}

	for {
		fmt.Print("agsh> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case line == "exit" || line == "quit":
			fmt.Println("Goodbye.")
			return
		case line == "help":
			printHelp(registry)
		case line == "commands":
			printCommands(registry)
		case strings.HasPrefix(line, "context "):
			handleContext(line, store)
		default:
			executeLine(line, executor, store, publisher)
		}
	}
}

func printHelp(registry *platform.Registry) {
	fmt.Println("Available commands:")
	fmt.Println("  help              Show this help message")
	fmt.Println("  commands          List all registered commands")
	fmt.Println("  context list      List context store contents")
	fmt.Println("  context get S K   Get a value from scope S, key K")
	fmt.Println("  context set S K V Set a value in scope S, key K")
	fmt.Println("  exit              Exit the shell")
	fmt.Println()
	fmt.Println("Pipeline syntax:")
	fmt.Println("  command1 arg | command2 arg   Pipe envelope between commands")
	fmt.Println()
	fmt.Println("Registered platform commands:")
	for _, cmd := range registry.List("") {
		fmt.Printf("  %-20s %s\n", cmd.Name(), cmd.Description())
	}
}

func printCommands(registry *platform.Registry) {
	cmds := registry.List("")
	if len(cmds) == 0 {
		fmt.Println("No commands registered.")
		return
	}
	for _, cmd := range cmds {
		fmt.Printf("  %-20s [%s] %s\n", cmd.Name(), cmd.Namespace(), cmd.Description())
	}
}

func handleContext(line string, store agshctx.ContextStore) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		fmt.Println("Usage: context [list|get|set] ...")
		return
	}

	switch parts[1] {
	case "list":
		scope := agshctx.ScopeSession
		if len(parts) >= 3 {
			scope = parts[2]
		}
		items, err := store.List(scope)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
		if len(items) == 0 {
			fmt.Printf("(empty scope: %s)\n", scope)
			return
		}
		for k, v := range items {
			fmt.Printf("  %s = %v\n", k, v)
		}
	case "get":
		if len(parts) < 4 {
			fmt.Println("Usage: context get <scope> <key>")
			return
		}
		val, err := store.Get(parts[2], parts[3])
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
		fmt.Printf("%v\n", val)
	case "set":
		if len(parts) < 5 {
			fmt.Println("Usage: context set <scope> <key> <value>")
			return
		}
		val := strings.Join(parts[4:], " ")
		if err := store.Set(parts[2], parts[3], val); err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
		fmt.Println("OK")
	default:
		fmt.Println("Usage: context [list|get|set] ...")
	}
}

func executeLine(line string, executor *registryExecutor, store agshctx.ContextStore, publisher *eventBusPublisher) {
	// Parse pipeline: command1 arg1 arg2 | command2 arg1
	segments := strings.Split(line, "|")

	steps := make([]agshctx.PipelineStep, 0, len(segments))
	for _, seg := range segments {
		parts := strings.Fields(strings.TrimSpace(seg))
		if len(parts) == 0 {
			continue
		}
		step := agshctx.PipelineStep{
			Command: parts[0],
		}
		if len(parts) > 1 {
			step.Args = parts[1:]
		}
		steps = append(steps, step)
	}

	if len(steps) == 0 {
		return
	}

	// Build input envelope from first command's args.
	var input agshctx.Envelope
	if len(steps[0].Args) > 0 {
		input = agshctx.NewEnvelope(strings.Join(steps[0].Args, " "), "text/plain", "repl")
		steps[0].Args = nil // Args consumed as payload.
	} else {
		input = agshctx.NewEnvelope(nil, "text/plain", "repl")
	}

	pipeline := &agshctx.Pipeline{
		Steps:    steps,
		Context:  store,
		Executor: executor,
		Events:   publisher,
	}

	ctx := gocontext.Background()
	result, err := pipeline.Run(ctx, input)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	// Display output.
	output := result.Output
	displayEnvelope(output)
}

func displayEnvelope(env agshctx.Envelope) {
	switch v := env.Payload.(type) {
	case string:
		fmt.Println(v)
	case []any:
		for _, item := range v {
			fmt.Printf("  %v\n", item)
		}
	default:
		// Pretty-print JSON for structured data.
		data, err := json.MarshalIndent(env.Payload, "", "  ")
		if err != nil {
			fmt.Println(env.PayloadString())
			return
		}
		fmt.Println(string(data))
	}
}
