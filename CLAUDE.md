# CLAUDE.md — Agent Briefing for agsh

## Project Overview

agsh (Agent Shell) is a CLI runtime for AI agents written in Go. Read the full architecture before writing any code.

## Architecture Docs (READ FIRST)

- `docs/architecture.md` — System architecture, all types, interfaces, project structure, build phases
- `docs/demo-specs.md` — 4 demo scenarios with specs, expected outputs, and test criteria
- `docs/inspector-gui.md` — Web-based inspector tool spec

These docs are the source of truth. If something is ambiguous, check the spec before improvising.

## Project Conventions

### Code Style
- Standard Go conventions (`gofmt`, `go vet`)
- Interfaces in the same file as the package-level types they define
- One package per directory, package name matches directory name
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`
- Tests alongside source files (`*_test.go`)

### Architecture Rules
- **Three pillars are independent packages.** `pkg/context`, `pkg/platform`, `pkg/verify` must not import each other directly. They compose through shared types defined in their own packages.
- **Shared types:** `pkg/context/envelope.go` defines the `Envelope` type used everywhere. Platform commands and verification engine both depend on it.
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
- Every package needs tests. At minimum: core types serialize/deserialize, interfaces have at least one integration test.
- Use table-driven tests where appropriate.
- Demo specs in `examples/demo/` serve as integration test fixtures.

### Git
- Commit after each logical unit of work (one package, one feature)
- Commit messages: `feat(context): implement Envelope type and serialization`
- Prefix: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

## Build & Run

```bash
# Build
go build -o bin/agsh ./cmd/agsh

# Test
go test ./...

# Docker
docker build -f docker/Dockerfile -t agsh .
docker-compose -f docker/docker-compose.yaml up

# Run
./bin/agsh                          # interactive mode
./bin/agsh --mode=agent             # agent mode (JSON-RPC stdin/stdout)
./bin/agsh run <spec.yaml>          # run a project spec
./bin/agsh --inspector              # enable web inspector
```

## Current Build Phase

Check which phase we're in and what's been completed:

### Phase 1: Foundation
- [x] Go module initialized, project structure scaffolded
- [x] `pkg/context/envelope.go` — Envelope type + serialization
- [x] `pkg/context/store.go` — ContextStore interface + bbolt implementation
- [x] `pkg/context/pipeline.go` — Pipeline definition + execution
- [x] `pkg/platform/command.go` — PlatformCommand interface
- [x] `pkg/platform/registry.go` — Command registry
- [x] `pkg/platform/fs/` — fs:list, fs:read, fs:write commands
- [x] `cmd/agsh/main.go` — Entrypoint with mode detection
- [x] `cmd/agsh/repl.go` — Basic interactive REPL
- [x] `pkg/events/` — EventBus (wire in from Phase 1 for inspector later)
- [x] `docker/Dockerfile` + `docker-compose.yaml`
- [x] Demo 01 runs end-to-end

### Phase 2: Platforms + Specs
- [ ] `pkg/platform/github/` — repo:info, pr:list, issue:create
- [ ] `pkg/platform/http/` — get, post with domain allowlisting
- [ ] `internal/config/` — YAML config loading (runtime + platforms)
- [ ] `pkg/spec/` — Spec types, loader, validator, planner
- [ ] `cmd/agsh/` — `run` and `init` subcommands
- [ ] `templates/` — At least 2 spec templates
- [ ] Demo 02 runs end-to-end

### Phase 3: Verification
- [ ] `pkg/verify/intent.go` — Intent + Assertion types
- [ ] `pkg/verify/engine.go` — VerificationEngine
- [ ] `pkg/verify/assertions.go` — Built-in assertion implementations
- [ ] `pkg/verify/checkpoint.go` — CheckpointManager
- [ ] Verification wired into pipeline execution
- [ ] Success criteria from specs feed into verification
- [ ] Demo 03 runs (both success and failure scenarios)

### Phase 4: Agent Mode + Inspector
- [ ] `pkg/protocol/` — JSON-RPC message types + handler
- [ ] `cmd/agsh/agent.go` — Agent mode (JSON-RPC over stdin/stdout)
- [ ] `project.*` methods in protocol (load, plan, approve, reject, run)
- [ ] `internal/inspector/` — HTTP server + WebSocket + embedded UI
- [ ] Approval flow works end-to-end
- [ ] Demo 04 runs with an LLM connected
- [ ] Inspector shows live pipeline progress
