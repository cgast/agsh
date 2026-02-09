# Agent Shell (agsh) — System Architecture & Build Spec

> A next-generation CLI runtime designed for AI agents: composable, context-aware,
> verified, and sandboxed. Think "bash for agents" — where commands carry context,
> pipelines carry state, remote platforms feel local, and every step verifies intent.

**Status:** Prototype / Proof of Concept  
**Language:** Go  
**Runtime:** Docker container (sandboxed, reproducible)  
**Codename:** `agsh` (agent shell)

---

## 1. Vision & Design Principles

### 1.1 Core Thesis

Bash gave humans composable power over the OS. `agsh` gives AI agents composable
power over everything — local files, remote services, other agents — with context,
state, and verification built into the runtime.

### 1.2 Design Principles

- **Agent-first, human-readable.** The primary user is an LLM. The secondary user
  is a human who wants to inspect, debug, and configure.
- **Composable over monolithic.** Small commands that combine, not one mega-tool.
- **Context flows everywhere.** Every command has access to shared context — not just
  stdin/stdout but structured metadata, goals, and history.
- **Verify, don't trust.** Every step can declare intent and the runtime checks
  outcomes before proceeding.
- **Platforms as commands.** Remote APIs feel like local CLI tools with consistent
  semantics.
- **Sandboxed by default.** Runs in a container. Filesystem, network, and resource
  access are controlled.

---

## 2. High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    LLM Orchestrator                      │
│              (external — talks to agsh via API)          │
└────────────────────────┬────────────────────────────────┘
                         │ JSON-RPC / stdin
┌────────────────────────▼────────────────────────────────┐
│                      agsh REPL                           │
│  ┌─────────────┐ ┌──────────────┐ ┌──────────────────┐  │
│  │   Context    │ │   Pipeline   │ │   Verification   │  │
│  │   Engine     │ │   Runtime    │ │   Engine         │  │
│  └──────┬──────┘ └──────┬───────┘ └────────┬─────────┘  │
│         │               │                  │             │
│  ┌──────▼───────────────▼──────────────────▼─────────┐  │
│  │                 Command Registry                   │  │
│  │  ┌──────────┐ ┌────────────┐ ┌─────────────────┐  │  │
│  │  │ Built-in │ │  Platform  │ │  User-defined   │  │  │
│  │  │ Commands │ │  Commands  │ │  Commands       │  │  │
│  │  └──────────┘ └────────────┘ └─────────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │                  State Store                       │  │
│  │    (context db, checkpoint history, run log)       │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                         │
              ┌──────────▼──────────┐
              │   Docker Container  │
              │   (sandboxed fs,    │
              │    controlled net)  │
              └─────────────────────┘
```

---

## 3. The Three Pillars

The codebase is split into three independent packages that compose through
well-defined interfaces. Each pillar can be developed and tested in isolation.

---

### 3.1 Pillar 1: Context-Aware Pipelines (`pkg/context`)

**Purpose:** Commands don't just pass text — they pass structured envelopes that
carry metadata, provenance, and project context alongside the payload.

#### 3.1.1 The Envelope

Every value flowing through a pipeline is an `Envelope`:

```go
// pkg/context/envelope.go
type Envelope struct {
    // The actual payload (text, JSON, binary reference)
    Payload   any            `json:"payload"`
    
    // Metadata about this data
    Meta      Metadata       `json:"meta"`
    
    // Trace of how this envelope was produced
    Provenance []Step        `json:"provenance"`
}

type Metadata struct {
    ContentType string            `json:"content_type"`  // "text/plain", "application/json", etc.
    Tags        map[string]string `json:"tags"`          // arbitrary k/v annotations
    CreatedAt   time.Time         `json:"created_at"`
    Source      string            `json:"source"`        // which command produced this
}

type Step struct {
    Command   string    `json:"command"`
    Args      []string  `json:"args"`
    Timestamp time.Time `json:"timestamp"`
    Duration  time.Duration `json:"duration"`
    Status    string    `json:"status"`  // "ok", "error", "skipped"
}
```

#### 3.1.2 The Context Store

A shared, scoped key-value store that all commands in a session can read/write:

```go
// pkg/context/store.go
type ContextStore interface {
    // Scoped access
    Get(scope, key string) (any, error)
    Set(scope, key string, value any) error
    Delete(scope, key string) error
    List(scope string) (map[string]any, error)
    
    // Predefined scopes
    // "project"  — goals, constraints, guidelines (loaded from config)
    // "session"  — current session state, working memory
    // "step"     — current pipeline step context (ephemeral)
    // "history"  — append-only log of all operations
}
```

Backed by an embedded key-value store (BoltDB/bbolt for the prototype).

#### 3.1.3 Pipeline Execution

Pipelines are defined as a sequence of commands. Unlike bash pipes, they pass
`Envelope` objects and have access to the `ContextStore`:

```go
// pkg/context/pipeline.go
type Pipeline struct {
    Steps    []PipelineStep
    Context  ContextStore
}

type PipelineStep struct {
    Command    string
    Args       []string
    Intent     string   // what this step is supposed to achieve (for verification)
    OnError    string   // "stop", "skip", "retry"
}
```

**Syntax (agent-facing):**

```
# Simple pipeline — context flows implicitly
files:list ./src | filter --type=go | analyze:complexity

# With intent declarations (feeds into Pillar 3)
files:list ./src 
  ?intent="get all Go source files"
  | filter --type=go 
  ?intent="keep only Go files"
  ?verify="output contains only .go files"
  | analyze:complexity
```

---

### 3.2 Pillar 2: Platform Commands (`pkg/platform`)

**Purpose:** Remote services are exposed as native CLI commands with consistent
semantics, discoverable via a registry, and composable in pipelines.

#### 3.2.1 Platform Command Interface

Every platform command implements a common interface:

```go
// pkg/platform/command.go
type PlatformCommand interface {
    // Identity
    Name() string                    // e.g. "github:pr:list"
    Description() string             // human/agent readable
    Namespace() string               // e.g. "github"
    
    // Schema
    InputSchema() Schema             // expected input (for validation + LLM guidance)
    OutputSchema() Schema            // output shape (for pipeline composition)
    
    // Execution
    Execute(ctx context.Context, input Envelope, store ContextStore) (Envelope, error)
    
    // Auth
    RequiredCredentials() []string   // e.g. ["GITHUB_TOKEN"]
}

type Schema struct {
    Type        string                 `json:"type"`
    Properties  map[string]SchemaField `json:"properties"`
    Required    []string               `json:"required"`
}

type SchemaField struct {
    Type        string `json:"type"`
    Description string `json:"description"`
}
```

#### 3.2.2 Command Registry

```go
// pkg/platform/registry.go
type Registry struct {
    commands map[string]PlatformCommand  // keyed by full name e.g. "github:pr:list"
}

func (r *Registry) Register(cmd PlatformCommand) error
func (r *Registry) Resolve(name string) (PlatformCommand, error)
func (r *Registry) List(namespace string) []PlatformCommand
func (r *Registry) Describe(name string) Schema  // for LLM tool discovery
```

#### 3.2.3 Prototype Platform Commands

For the prototype, implement 2-3 real platform integrations:

| Command | Description |
|---------|-------------|
| `fs:list`, `fs:read`, `fs:write` | Local filesystem (sandboxed to workdir) |
| `github:repo:info`, `github:pr:list`, `github:issue:create` | GitHub API |
| `http:get`, `http:post` | Generic HTTP (allowlisted domains) |

Each namespace lives in its own sub-package: `pkg/platform/fs/`, `pkg/platform/github/`, etc.

#### 3.2.4 Platform Config

Credentials and platform-specific config loaded from a standard location:

```yaml
# /workspace/.agsh/platforms.yaml
github:
  token: "${GITHUB_TOKEN}"
  default_owner: "cgast"
http:
  allowed_domains:
    - "api.github.com"
    - "httpbin.org"
```

---

### 3.3 Pillar 3: Verified Execution (`pkg/verify`)

**Purpose:** Every command can declare what it intends to do and what success
looks like. The runtime verifies outcomes before proceeding.

#### 3.3.1 Intent & Assertion Model

```go
// pkg/verify/intent.go
type Intent struct {
    Description string       `json:"description"`   // natural language intent
    Assertions  []Assertion  `json:"assertions"`     // machine-checkable conditions
}

type Assertion struct {
    Type     string `json:"type"`      // "contains", "not_empty", "json_schema", 
                                        // "count_gte", "matches_regex", "llm_judge"
    Target   string `json:"target"`    // what to check: "output", "context.session.x", "meta.tags.y"
    Expected any    `json:"expected"`  // the expected value/pattern
    Message  string `json:"message"`   // human-readable failure description
}
```

#### 3.3.2 Verification Engine

```go
// pkg/verify/engine.go
type VerificationEngine interface {
    // Verify an envelope against a set of assertions
    Verify(envelope Envelope, intent Intent) (VerificationResult, error)
}

type VerificationResult struct {
    Passed     bool                `json:"passed"`
    Results    []AssertionResult   `json:"results"`
    Timestamp  time.Time           `json:"timestamp"`
}

type AssertionResult struct {
    Assertion  Assertion `json:"assertion"`
    Passed     bool      `json:"passed"`
    Actual     any       `json:"actual"`
    Message    string    `json:"message"`
}
```

#### 3.3.3 Built-in Assertion Types

| Type | Description | Example |
|------|-------------|---------|
| `not_empty` | Output payload is not empty | `?verify="not_empty"` |
| `contains` | Output contains substring | `?verify="contains:.go"` |
| `count_gte` | Array/line count >= N | `?verify="count_gte:5"` |
| `json_schema` | Output matches JSON schema | `?verify="json_schema:{...}"` |
| `matches_regex` | Output matches regex | `?verify="matches_regex:\\d+"` |
| `llm_judge` | Ask an LLM if the output matches intent | `?verify="llm_judge"` |

The `llm_judge` type is powerful for the prototype — it sends the intent
description + output to an LLM and asks "does this output satisfy the intent?"
This bridges the gap between fuzzy human goals and machine-checkable conditions.

#### 3.3.4 Checkpointing

The verification engine also manages checkpoints so pipelines can be rolled back:

```go
// pkg/verify/checkpoint.go
type CheckpointManager interface {
    Save(name string, state SessionSnapshot) error
    Restore(name string) (SessionSnapshot, error)
    List() ([]CheckpointInfo, error)
    Diff(a, b string) ([]Change, error)
}

type SessionSnapshot struct {
    ContextState  map[string]map[string]any  // full context store dump
    WorkdirHash   string                      // hash of working directory state
    Timestamp     time.Time
}
```

---

## 4. Human-Agent Interaction Model

The runtime is only useful if there's a clear way for humans to give agents work
and steer execution. `agsh` supports three interaction patterns, all converging
on a single source of truth: the **project spec file**.

### 4.1 The Project Spec (`project.agsh.yaml`)

Every task or project is defined by a spec file. This is the contract between
human intent and agent execution. The agent reads it, plans against it, and
verifies outcomes against its success criteria.

```yaml
# project.agsh.yaml — the source of truth for any task
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "weekly-github-report"
  description: "Generate a weekly summary of GitHub activity across my repos"
  author: "cgast"
  created: "2025-02-09"
  tags: ["reporting", "github", "weekly"]

# What the agent should achieve
goal: |
  Generate a markdown report summarizing all GitHub activity across my
  repositories for the past 7 days. The report should be useful for a
  weekly standup or status update.

# Hard constraints the agent must respect
constraints:
  - "Only include repos owned by the authenticated user"
  - "Do not create, modify, or delete any GitHub resources"
  - "Output must be a single markdown file"
  - "Must complete within 60 seconds"

# Soft guidelines for quality and style
guidelines:
  - "Group activity by repository, then by type (PRs, issues, commits)"
  - "Keep summaries to 1-2 sentences per item"
  - "Include links to relevant GitHub pages"
  - "Flag any PRs open longer than 7 days"

# Machine-checkable success criteria (fed into verification engine)
success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Report must not be empty"
  - type: "contains"
    target: "output"
    expected: "## "
    message: "Report must contain markdown headers"
  - type: "llm_judge"
    target: "output"
    expected: "The report covers GitHub activity from the last 7 days, grouped by repo"
    message: "Report must match the stated goal"

# Resources the agent is allowed to use
allowed_commands:
  - "github:*"          # all github commands
  - "fs:write"          # to write the output file
  - "http:get"          # for fetching additional data if needed

# Output expectations
output:
  path: "./reports/weekly-{{date}}.md"
  format: "markdown"

# Optional: variables the human provides at runtime
params:
  - name: "date_range_days"
    type: "integer"
    default: 7
    description: "How many days back to look"
  - name: "repos"
    type: "string[]"
    default: []
    description: "Specific repos to include (empty = all owned repos)"
```

#### 4.1.1 Spec Schema (`pkg/spec`)

```go
// pkg/spec/spec.go
type ProjectSpec struct {
    APIVersion      string            `yaml:"apiVersion"`
    Kind            string            `yaml:"kind"`
    Meta            SpecMeta          `yaml:"meta"`
    Goal            string            `yaml:"goal"`
    Constraints     []string          `yaml:"constraints"`
    Guidelines      []string          `yaml:"guidelines"`
    SuccessCriteria []Assertion       `yaml:"success_criteria"` // reuses verify.Assertion
    AllowedCommands []string          `yaml:"allowed_commands"` // glob patterns
    Output          OutputSpec        `yaml:"output"`
    Params          []ParamDef        `yaml:"params"`
}

type SpecMeta struct {
    Name        string   `yaml:"name"`
    Description string   `yaml:"description"`
    Author      string   `yaml:"author"`
    Created     string   `yaml:"created"`
    Tags        []string `yaml:"tags"`
}

type OutputSpec struct {
    Path   string `yaml:"path"`
    Format string `yaml:"format"`
}

type ParamDef struct {
    Name        string `yaml:"name"`
    Type        string `yaml:"type"`
    Default     any    `yaml:"default"`
    Description string `yaml:"description"`
}
```

### 4.2 Three Ways to Start Work

#### 4.2.1 Direct Spec (declarative — human writes the spec)

The human creates or edits `project.agsh.yaml` directly. Then:

```bash
# In interactive mode
agsh run project.agsh.yaml

# Or in agent mode (JSON-RPC)
{"method": "project.run", "params": {"spec": "project.agsh.yaml"}}
```

The agent loads the spec, produces an execution plan, and (depending on config)
either executes immediately or asks for approval first.

#### 4.2.2 Conversational Bootstrap (human describes, agent specs)

The human describes what they want in natural language. The agent drafts a
`project.agsh.yaml`, presents it for review, and waits for approval:

```
Human: "I want a weekly report of my GitHub activity"

Agent: Here's a project spec I've drafted:
       [shows project.agsh.yaml]
       
       Shall I run this, or would you like to adjust anything?
```

The key insight: the conversation is an input method, but the spec file is the
artifact. Specs are saved, versioned, and reusable. Conversations are ephemeral.

#### 4.2.3 Template Init (scaffolded start)

Pre-built spec templates for common workflows:

```bash
agsh init --template=code-review
agsh init --template=weekly-report
agsh init --template=deploy-pipeline
agsh init --template=research
```

This creates a `project.agsh.yaml` with sensible defaults that the human fills in.
Templates live in the `examples/` directory and can be user-extended.

### 4.3 The Approval & Execution Lifecycle

Before executing, the agent always produces a **plan** — a concrete sequence of
commands derived from the spec. Depending on the `approval_mode` in config, the
human can review before execution.

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Load Spec  │────▶│  Plan Steps  │────▶│   Approve?   │────▶│   Execute    │
│              │     │              │     │  (if config)  │     │  + Verify    │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
                                                │                     │
                                           Human edits           On failure:
                                           plan/spec             retry, skip,
                                                                 or rollback
```

#### 4.3.1 Approval Modes

```yaml
# In .agsh/config.yaml
approval:
  mode: "plan"        # "always" | "plan" | "destructive" | "never"
  # always:      approve every command before execution
  # plan:        approve the full plan, then auto-execute
  # destructive: auto-execute reads, approve writes/creates/deletes
  # never:       full autonomy (for trusted, well-tested specs)
```

#### 4.3.2 Plan Output

The plan is a structured preview of what the agent intends to do:

```json
{
    "spec": "weekly-github-report",
    "steps": [
        {
            "command": "github:repo:list",
            "args": {"affiliation": "owner"},
            "intent": "Get all repos I own",
            "risk": "read-only",
            "verify": [{"type": "not_empty"}]
        },
        {
            "command": "github:pr:list",
            "args": {"state": "all", "since": "{{7_days_ago}}"},
            "intent": "Get PRs from the last 7 days for each repo",
            "risk": "read-only",
            "verify": [{"type": "json_schema", "expected": "..."}]
        },
        {
            "command": "fs:write",
            "args": {"path": "./reports/weekly-2025-02-09.md"},
            "intent": "Write the final report",
            "risk": "write",
            "checkpoint_before": true,
            "verify": [{"type": "not_empty"}, {"type": "contains", "expected": "## "}]
        }
    ],
    "estimated_duration": "~15s",
    "risk_summary": "2 read-only API calls, 1 local file write"
}
```

The human can approve, edit, or reject. Edits to the plan can optionally be
saved back to the spec for future runs.

### 4.4 Agent Mode Protocol Extensions

Additional JSON-RPC methods for the interaction model:

| Method | Purpose |
|--------|---------|
| `project.load` | Load a spec file, return parsed spec |
| `project.run` | Load + plan + (approve) + execute a spec |
| `project.plan` | Generate a plan from a spec without executing |
| `project.approve` | Approve a pending plan for execution |
| `project.reject` | Reject a plan, optionally with feedback |
| `project.init` | Scaffold a new spec from a template |
| `project.validate` | Check a spec for errors without running |

---

## 5. The Shell Runtime (`cmd/agsh`)

### 5.1 REPL

The main binary is a REPL that accepts commands in two modes:

1. **Interactive mode** — human types commands, sees formatted output
2. **Agent mode** — reads JSON-RPC from stdin, writes JSON-RPC to stdout
   (this is what an LLM orchestrator connects to)

```go
// cmd/agsh/main.go
func main() {
    // Detect mode
    if isTerminal(os.Stdin) {
        runInteractiveREPL()
    } else {
        runAgentMode()  // JSON-RPC over stdin/stdout
    }
}
```

### 5.2 Agent Mode Protocol

When an LLM connects, it communicates via simple JSON-RPC:

```json
// Request
{
    "method": "execute",
    "params": {
        "command": "github:pr:list",
        "args": {"repo": "cgast/agsh", "state": "open"},
        "intent": "Get all open PRs for the agsh repo",
        "verify": [{"type": "not_empty"}]
    }
}

// Response
{
    "result": {
        "payload": [...],
        "meta": {"content_type": "application/json", "source": "github:pr:list"},
        "verification": {"passed": true, "results": [...]},
        "provenance": [...]
    }
}
```

Additional methods:

| Method | Purpose |
|--------|---------|
| `execute` | Run a single command |
| `pipeline` | Run a multi-step pipeline |
| `context.get` / `context.set` | Read/write context store |
| `commands.list` | Discover available commands |
| `commands.describe` | Get schema for a command |
| `checkpoint.save` / `checkpoint.restore` | Manage checkpoints |
| `history` | Get execution history |

### 5.3 Built-in Commands

Beyond platform commands, `agsh` includes shell-level built-ins:

| Command | Purpose |
|---------|---------|
| `help` | List all commands or describe one |
| `context` | View/edit context store |
| `checkpoint` | Save/restore/list checkpoints |
| `history` | View execution log |
| `plan` | Declare a multi-step plan (stored in context) |
| `verify` | Manually verify last output against assertions |
| `config` | View/edit runtime config |
| `!bash` | Escape hatch — run raw bash (logged, audited) |

---

## 6. Project Structure

```
agsh/
├── cmd/
│   └── agsh/
│       ├── main.go              # entrypoint, mode detection
│       ├── repl.go              # interactive REPL
│       └── agent.go             # JSON-RPC agent mode
│
├── pkg/
│   ├── context/                 # PILLAR 1: Context-aware pipelines
│   │   ├── envelope.go          # Envelope type definition
│   │   ├── store.go             # ContextStore interface + bbolt impl
│   │   ├── pipeline.go          # Pipeline definition & execution
│   │   └── pipeline_test.go
│   │
│   ├── platform/                # PILLAR 2: Platform commands
│   │   ├── command.go           # PlatformCommand interface
│   │   ├── registry.go          # Command registry
│   │   ├── schema.go            # Schema types
│   │   ├── fs/                  # filesystem commands
│   │   │   ├── list.go
│   │   │   ├── read.go
│   │   │   └── write.go
│   │   ├── github/              # github commands
│   │   │   ├── repo_info.go
│   │   │   ├── pr_list.go
│   │   │   └── issue_create.go
│   │   └── http/                # generic http commands
│   │       ├── get.go
│   │       └── post.go
│   │
│   ├── verify/                  # PILLAR 3: Verified execution
│   │   ├── intent.go            # Intent & Assertion types
│   │   ├── engine.go            # VerificationEngine
│   │   ├── assertions.go        # built-in assertion implementations
│   │   ├── checkpoint.go        # CheckpointManager
│   │   └── engine_test.go
│   │
│   ├── spec/                    # Project spec loading & validation
│   │   ├── spec.go              # ProjectSpec types
│   │   ├── loader.go            # YAML loading + variable interpolation
│   │   ├── validator.go         # Spec validation (required fields, command globs)
│   │   └── planner.go           # Spec → ExecutionPlan conversion
│   │
│   └── protocol/                # Agent communication protocol
│       ├── jsonrpc.go           # JSON-RPC message types
│       └── handler.go           # Request routing
│
├── internal/
│   ├── config/                  # Configuration loading
│   │   └── config.go
│   └── sandbox/                 # Sandbox enforcement (fs restrictions etc.)
│       └── sandbox.go
│
├── docker/
│   ├── Dockerfile               # Multi-stage build
│   └── docker-compose.yaml      # Dev environment with volume mounts
│
├── templates/                   # Spec templates for `agsh init --template=X`
│   ├── code-review.yaml
│   ├── weekly-report.yaml
│   ├── deploy-pipeline.yaml
│   └── research.yaml
│
├── examples/
│   ├── specs/                   # Example project specs (reference/learning)
│   │   ├── hello-world.agsh.yaml
│   │   ├── github-weekly-report.agsh.yaml
│   │   └── verified-file-transform.agsh.yaml
│   ├── pipelines/               # Raw pipeline examples (no spec wrapper)
│   │   ├── hello-pipeline.json
│   │   ├── github-workflow.json
│   │   └── verified-deploy.json
│   └── demo/                    # Runnable demo scenarios for testing
│       ├── README.md            # How to run demos
│       ├── 01-basic-pipeline/   # Simplest possible pipeline
│       │   ├── project.agsh.yaml
│       │   └── workspace/       # pre-populated test files
│       ├── 02-github-report/    # Platform commands + real API
│       │   ├── project.agsh.yaml
│       │   └── expected-output/ # for verification comparison
│       ├── 03-verified-transform/ # Verification engine showcase
│       │   ├── project.agsh.yaml
│       │   └── workspace/
│       └── 04-agent-autonomy/   # Full agent loop: discover → plan → execute → verify
│           ├── project.agsh.yaml
│           └── orchestrator.sh  # Script that connects an LLM to agsh
│
├── .agsh/
│   ├── config.yaml              # Default runtime config
│   └── platforms.yaml           # Platform credentials (gitignored)
│
├── go.mod
├── go.sum
├── Makefile                     # build, test, docker-build, docker-run
└── README.md
```

---

## 7. Docker Setup

### 7.1 Dockerfile

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /agsh ./cmd/agsh

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache git bash ca-certificates
COPY --from=builder /agsh /usr/local/bin/agsh

# Sandbox workspace
RUN mkdir -p /workspace /data
WORKDIR /workspace

# Non-root user for sandbox
RUN adduser -D -h /workspace agsh
USER agsh

ENTRYPOINT ["agsh"]
```

### 7.2 docker-compose.yaml

```yaml
version: "3.8"
services:
  agsh:
    build:
      context: .
      dockerfile: docker/Dockerfile
    volumes:
      - ./workspace:/workspace    # shared workspace
      - agsh-data:/data           # persistent state (context store, checkpoints)
    environment:
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - AGSH_MODE=agent           # or "interactive"
    stdin_open: true
    tty: true
    # Network restrictions for sandbox
    networks:
      - agsh-net

  # Optional: for testing agent mode with a simple LLM proxy
  # orchestrator:
  #   image: ...
  #   depends_on: [agsh]

volumes:
  agsh-data:

networks:
  agsh-net:
    driver: bridge
```

---

## 8. Build Order & Milestones

### Phase 1: Foundation (Week 1)

**Goal:** Basic REPL + Envelope model + Context Store working.

1. Initialize Go module, set up project structure
2. Implement `Envelope` type and serialization
3. Implement `ContextStore` with bbolt backend
4. Build basic REPL loop (interactive mode)
5. Implement `fs:list`, `fs:read`, `fs:write` as first commands
6. Wire up simple pipeline execution (no verification yet)
7. Docker build + run works

**Demo:** `fs:list /workspace | fs:read` with envelopes flowing between steps.

### Phase 2: Platforms + Spec Loading (Week 2)

**Goal:** Platform commands work, registry is functional, specs can be loaded.

1. Implement `Registry` with namespace resolution
2. Add `github:repo:info`, `github:pr:list`, `github:issue:create`
3. Add `http:get`, `http:post` with domain allowlisting
4. Implement platform config loading from YAML
5. Add `commands.list` and `commands.describe` for discoverability
6. Implement `pkg/spec` — spec parsing, validation, and plan generation
7. Implement `agsh run <spec>` and `agsh init --template=<name>`
8. Create first demo specs in `examples/demo/`

**Demo:** `agsh run examples/demo/02-github-report/project.agsh.yaml` loads
the spec, generates a plan, and executes (with approval prompt).

### Phase 3: Verification (Week 3)

**Goal:** Verified execution with assertions and checkpoints.

1. Implement assertion types: `not_empty`, `contains`, `count_gte`, `matches_regex`, `json_schema`
2. Wire verification into pipeline execution
3. Implement `llm_judge` assertion type (calls external LLM)
4. Implement checkpoint save/restore
5. Add intent tracking to execution history
6. Wire `success_criteria` from spec into verification engine

**Demo:** Run `examples/demo/03-verified-transform/` — pipeline that transforms
files, verifies the output matches criteria, and rolls back on failure.

### Phase 4: Agent Mode (Week 4)

**Goal:** LLM can connect and drive `agsh` autonomously.

1. Implement JSON-RPC protocol handler
2. Implement all agent-mode methods including `project.*` methods
3. Implement approval flow (plan → approve → execute) over JSON-RPC
4. Add `plan` command for multi-step plan declaration
5. Test with Claude/GPT connecting via stdin
6. Build `examples/demo/04-agent-autonomy/` end-to-end demo

**Demo:** An LLM connects, loads a spec, reviews the plan, executes with
verification, and handles a failure by rolling back and retrying.

---

## 9. Key Dependencies (Go Modules)

```
go.etcd.io/bbolt          # embedded key-value store for context
github.com/google/go-github/v60  # GitHub API client
github.com/spf13/cobra    # CLI framework (optional, for interactive mode)
github.com/fatih/color    # terminal colors for interactive mode
gopkg.in/yaml.v3          # config parsing
github.com/stretchr/testify  # testing
```

---

## 10. Configuration

### 10.1 Runtime Config (`.agsh/config.yaml`)

```yaml
# Runtime behavior
mode: interactive    # "interactive" or "agent"
log_level: info

# Sandbox
sandbox:
  workdir: /workspace
  allowed_paths:
    - /workspace
    - /tmp
  denied_paths:
    - /etc
    - /usr
  max_file_size: 10MB

# Approval (see Section 4.3.1)
approval:
  mode: plan           # "always" | "plan" | "destructive" | "never"
  timeout: 300         # seconds to wait for human approval before aborting

# Verification defaults
verify:
  fail_fast: true              # stop pipeline on first verification failure
  llm_judge_endpoint: ""       # optional: LLM endpoint for llm_judge assertions
  llm_judge_model: ""          # optional: model to use

# History
history:
  max_entries: 10000
  persist: true
```

---

## 11. Success Criteria for the Prototype

The prototype is "done" when:

1. **A human can write a spec** (`project.agsh.yaml`) that fully describes a task
2. **`agsh run` loads the spec**, generates an execution plan, and presents it for approval
3. **An LLM can connect** to `agsh` via JSON-RPC and discover available commands
4. **It can compose a pipeline** that spans local files + a remote service (GitHub)
5. **Context flows** between steps — a later command can reference data set by an earlier one
6. **Verification catches a failure** — e.g. "this file should contain X" fails and the pipeline stops
7. **Checkpoint/rollback works** — the agent can save state before a risky operation and restore it
8. **Everything runs in Docker** — reproducible, sandboxed, one `docker-compose up` to start
9. **4 demo scenarios** in `examples/demo/` all run successfully end-to-end

---

## 12. Future Directions (Out of Scope for Prototype)

- **MCP compatibility layer** — expose `agsh` commands as MCP tools
- **Agent-to-agent communication** — one `agsh` instance delegates to another
- **Plugin system** — load platform commands from external binaries/WASM
- **Collaborative mode** — multiple agents share a context store
- **Natural language command parsing** — agent says "list the open PRs" and it resolves to `github:pr:list --state=open`
- **Cost/token budgeting** — track and limit LLM API costs within pipelines
- **Workspace templates** — preconfigured contexts for common workflows (e.g. "code review", "deploy", "research")
