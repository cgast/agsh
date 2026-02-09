# CLAUDE.md — Agent Briefing for agsh

## Project Overview

agsh (Agent Shell) is a CLI runtime for AI agents written in Go. Read the full architecture before writing any code.

## Architecture Docs (READ FIRST)

- `docs/architecture.md` — System architecture, all types, interfaces, project structure, build phases
- `docs/demo-specs.md` — 4 demo scenarios with specs, expected outputs, and test criteria
- `docs/inspector-gui.md` — Web-based inspector tool spec

These docs are the source of truth. If something is ambiguous, check the spec before improvising.

## Task Workflow

This project uses Claude Code Tasks for coordination. When starting a phase:

1. Read the phase description in this file
2. Create Tasks with dependencies using TaskCreate
3. Work through tasks in dependency order
4. Mark tasks complete as you go
5. Run `go test ./...` before marking any implementation task complete

When working in parallel sessions (via `CLAUDE_CODE_TASK_LIST_ID`):
- Check TaskList() for available (pending, unowned, unblocked) tasks
- Claim a task before starting work on it
- Don't touch files owned by another session's active task
- Commit after completing each task

## Project Conventions

### Code Style
- Standard Go conventions (`gofmt`, `go vet`)
- Interfaces in the same file as the package-level types they define
- One package per directory, package name matches directory name
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`
- Tests alongside source files (`*_test.go`)

### Architecture Rules
- **Three pillars are independent packages.** `pkg/context`, `pkg/platform`, `pkg/verify` must not import each other directly. They compose through shared types.
- **Shared types:** `pkg/context/envelope.go` defines the `Envelope` type used everywhere.
- **No circular imports.** If two packages need to talk, introduce an interface in the consumer.
- **`internal/` is for implementation details** that shouldn't be imported outside this module.
- **`cmd/agsh/` is thin.** It wires things together but contains minimal logic.

### Dependency Policy
- Minimize external deps. Prefer stdlib where reasonable.
- Approved deps (see `docs/architecture.md` Section 9):
  - `go.etcd.io/bbolt` — embedded KV store
  - `github.com/google/go-github/v60` — GitHub API
  - `gopkg.in/yaml.v3` — config parsing
  - `github.com/stretchr/testify` — testing
  - `github.com/fatih/color` — terminal output
  - `github.com/spf13/cobra` — CLI framework (optional)

### File Layout
Follow the structure in `docs/architecture.md` Section 6 exactly. Don't reorganize.

### Testing
- Every package needs tests
- Use table-driven tests where appropriate
- Demo specs in `examples/demo/` serve as integration test fixtures

### Git
- Commit after each logical unit of work
- Commit messages: `feat(context): implement Envelope type and serialization`
- Prefix: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

## Build & Run

```bash
go build -o bin/agsh ./cmd/agsh
go test ./...
docker build -f docker/Dockerfile -t agsh .
docker-compose -f docker/docker-compose.yaml up
```

## Phase Definitions

### Phase 1: Foundation ✅ COMPLETE

### Phase 2: Platforms + Specs ✅ COMPLETE

Create these as Tasks with dependencies. T1-T3 and T5 can run in parallel.

| Task | Description | Blocked By |
|------|-------------|------------|
| T1 | `pkg/platform/github/` — repo:info, pr:list, issue:create | — |
| T2 | `pkg/platform/http/` — get, post with domain allowlisting | — |
| T3 | `internal/config/` — YAML config loading, platform credentials | — |
| T4 | Wire platform config into Registry, register all commands | T1, T2, T3 |
| T5 | `pkg/spec/spec.go` — ProjectSpec types | — |
| T6 | `pkg/spec/loader.go` — YAML loading + variable interpolation | T5 |
| T7 | `pkg/spec/validator.go` — spec validation | T5 |
| T8 | `pkg/spec/planner.go` — spec → execution plan | T5, T6, T7 |
| T9 | `run` and `init` subcommands in cmd/agsh | T4, T8 |
| T10 | Create templates/ + Demo 02 spec and workspace | T9 |
| T11 | Review: verify Phase 2 against docs/architecture.md | T10 |

### Phase 3: Verification ✅ COMPLETE

| Task | Description | Blocked By |
|------|-------------|------------|
| T1 | `pkg/verify/intent.go` + `assertions.go` — types + built-in assertions | — |
| T2 | `pkg/verify/engine.go` — VerificationEngine | T1 |
| T3 | `llm_judge` assertion type (optional, skip if no endpoint) | T2 |
| T4 | `pkg/verify/checkpoint.go` — CheckpointManager | — |
| T5 | Wire verification into pipeline execution | T2, T4 |
| T6 | Wire success_criteria from specs into verification | T5 |
| T7 | Demo 03 — both success and failure paths | T6 |
| T8 | Review: verify Phase 3 against docs/architecture.md | T7 |

### Phase 4: Agent Mode + Inspector

T1-T5 (protocol) and T6-T8 (inspector) can run in parallel.

| Task | Description | Blocked By |
|------|-------------|------------|
| T1 | `pkg/protocol/jsonrpc.go` — JSON-RPC types | — |
| T2 | `pkg/protocol/handler.go` — method routing | T1 |
| T3 | `cmd/agsh/agent.go` — agent mode loop | T2 |
| T4 | `project.*` methods — load, plan, approve, reject, run | T3 |
| T5 | Approval flow end-to-end | T4 |
| T6 | `pkg/events/` — ensure all runtime events are emitted | — |
| T7 | `internal/inspector/server.go` — HTTP + WebSocket server | T6 |
| T8 | `internal/inspector/ui/` — frontend | T7 |
| T9 | Wire inspector into main.go with --inspector flag | T7, T8 |
| T10 | Demo 04 — full agent autonomy test | T5, T9 |
| T11 | Review: verify Phase 4 against spec | T10 |
