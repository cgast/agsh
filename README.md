# agsh — Agent Shell

> Bash gave humans composable power over the OS.  
> `agsh` gives AI agents composable power over everything.

**agsh** is a CLI runtime designed for AI agents. It replaces the flat text pipes of traditional shells with context-aware pipelines, treats remote platforms as local commands, and verifies every step against declared intent — all inside a sandboxed container.

The premise: as AI agents become the primary operators of software, the tools they work with should be designed for them — not adapted from human GUIs or bolted onto legacy APIs. agsh is an exploration of what that looks like in practice.

---

## Core Ideas

### Context-Aware Pipelines

In bash, data flows as unstructured text. In agsh, data flows as **envelopes** — typed payloads with metadata, tags, and a full provenance chain. Every command in a pipeline can read and write to a shared context store, so downstream steps know what happened upstream.

### Platform Commands

Remote services feel like local CLI tools. `github:pr:list`, `http:get`, `fs:read` — all share a consistent interface, are discoverable by the agent, and compose naturally in pipelines. Adding a new platform is implementing a Go interface and registering it.

### Verified Execution

Every pipeline step can declare intent ("get all open PRs") and success criteria ("output is not empty and contains JSON"). The runtime checks assertions before proceeding. If verification fails, execution stops and rolls back to the last checkpoint. An `llm_judge` assertion type can even ask an LLM whether the output matches the goal.

### Project Specs

Humans define *what* to do in a `project.agsh.yaml` — goals, constraints, guidelines, success criteria. The agent figures out *how*, generates a plan, and optionally waits for approval before executing. The spec is the contract between human intent and agent execution.

### Inspector GUI

A built-in web UI (`localhost:4200`) lets humans observe what the agent is doing in real time: live event stream, pipeline progress, context state, checkpoint history, and plan approval — without reading JSON-RPC logs.

---

## Status

**Early prototype.** This project is in the architecture and initial build phase. The design docs are solid; the code is catching up.

---

## Quick Start

```bash
# Build and run in Docker
docker-compose up -d --build

# Run a demo spec
docker-compose exec agsh agsh run examples/demo/01-basic-pipeline/project.agsh.yaml

# Open the inspector
# http://localhost:4200

# Run in agent mode (JSON-RPC over stdin/stdout for LLM integration)
docker-compose exec -T agsh agsh --mode=agent
```

---

## Project Structure

```
agsh/
├── cmd/agsh/              # CLI entrypoint, REPL, agent mode
├── pkg/
│   ├── context/           # Envelopes, context store, pipeline execution
│   ├── platform/          # Platform command interface + implementations
│   │   ├── fs/            #   filesystem commands
│   │   ├── github/        #   GitHub API commands
│   │   └── http/          #   generic HTTP commands
│   ├── verify/            # Assertions, verification engine, checkpoints
│   ├── spec/              # Project spec loading, validation, planning
│   ├── events/            # Event bus for runtime observability
│   └── protocol/          # JSON-RPC agent communication protocol
├── internal/
│   ├── config/            # Configuration loading
│   ├── sandbox/           # Filesystem and network restrictions
│   └── inspector/         # Built-in web UI
├── templates/             # Spec templates for `agsh init`
├── examples/demo/         # Runnable demo scenarios
├── docker/                # Dockerfile, docker-compose
└── docs/                  # Architecture specs
```

---

## Architecture

Full design documents:

- **[System Architecture](docs/architecture.md)** — the three pillars (context, platforms, verification), shell runtime, project specs, interaction model, and build phases
- **[Demo Specs](docs/demo-specs.md)** — four progressive demo scenarios from basic pipelines to full agent autonomy
- **[Inspector GUI](docs/inspector-gui.md)** — the web-based observation and debugging tool

---

## How It Works

### 1. Write a spec

```yaml
# project.agsh.yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "weekly-report"

goal: |
  Summarize GitHub activity across my repos for the last 7 days.

constraints:
  - "Read-only GitHub access"
  - "Output as markdown"

success_criteria:
  - type: "not_empty"
    target: "output"
  - type: "llm_judge"
    target: "output"
    expected: "A weekly report grouped by repo with PRs and issues"

allowed_commands:
  - "github:*"
  - "fs:write"

output:
  path: "./reports/weekly-{{date}}.md"
  format: "markdown"
```

### 2. Run it

```bash
agsh run project.agsh.yaml
```

### 3. Review the plan

```
Plan: weekly-report (3 steps, est. ~15s)
  [1] github:repo:list          read-only
  [2] github:pr:list (×N)       read-only
  [3] fs:write report.md        write (checkpoint before)

Risk: low — 2 read-only API calls, 1 local file write
Approve? [y/n]
```

### 4. Watch it run

Open `http://localhost:4200` to see real-time progress, or watch the terminal output.

---

## Agent Mode

For LLM integration, agsh speaks JSON-RPC over stdin/stdout:

```json
{"method": "commands.list"}
{"method": "project.load", "params": {"spec": "project.agsh.yaml"}}
{"method": "project.plan"}
{"method": "project.approve"}
{"method": "execute", "params": {"command": "github:pr:list", "args": {"repo": "cgast/agsh"}}}
{"method": "checkpoint.save", "params": {"name": "pre-write"}}
```

The agent discovers commands, loads specs, generates plans, executes with verification, and manages checkpoints — all through a clean protocol designed for LLM consumption.

---

## Tech

- **Language:** Go
- **Runtime:** Docker (sandboxed, reproducible)
- **State:** bbolt (embedded key-value store)
- **Inspector:** Embedded web UI via `go:embed`
- **Protocol:** JSON-RPC over stdin/stdout

---

## Contributing

This is an early-stage exploration project. If the ideas resonate, open an issue to discuss before sending PRs. The architecture docs in `docs/` are the best starting point for understanding the design.

---

## License

MIT
