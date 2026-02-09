# agsh Inspector — Human Insight GUI

> A lightweight web-based tool for humans to observe, debug, and understand
> what an agent is doing inside agsh. Not a control plane — just a window
> into the runtime with enough interactivity to inspect and steer.

**Status:** Prototype / Companion to agsh  
**Tech:** Go backend (embedded in agsh binary) + single-page HTML/JS frontend  
**Runs:** Served by agsh on a local port (e.g. `http://localhost:4200`)

---

## 1. Design Philosophy

The inspector exists because **agents are opaque by default**. When an LLM
is autonomously executing pipelines, the human needs to answer:

- What is it doing right now?
- What has it done so far?
- Why did it make that decision?
- What data is it working with?
- Did anything go wrong?
- Can I intervene?

The inspector answers all of these without requiring the human to read
JSON-RPC logs or parse terminal output.

### 1.1 Principles

- **Read-heavy, write-light.** Primarily an observation tool. Limited
  intervention (approve/reject plans, pause execution). Not a full IDE.
- **Real-time by default.** Streams updates via WebSocket. No polling.
- **Zero config.** Starts automatically with `agsh --inspector` or
  `agsh --inspector-port=4200`. No separate install.
- **Embedded, not separate.** Built into the agsh binary. The Go backend
  serves the frontend as embedded static assets. One binary, one process.
- **Works on mobile.** The human might be checking from their phone while
  the agent runs on a server. Responsive layout, simple UI.

---

## 2. Architecture

```
┌─────────────────────────────────────────────┐
│                agsh process                  │
│                                              │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │  REPL /  │  │ Inspector│  │  Runtime   │  │
│  │  Agent   │  │  HTTP    │  │  Engine    │  │
│  │  Mode    │  │  Server  │  │           │  │
│  └────┬─────┘  └────┬─────┘  └─────┬─────┘  │
│       │              │              │         │
│  ┌────▼──────────────▼──────────────▼──────┐  │
│  │            Event Bus                     │  │
│  │  (internal pub/sub for runtime events)   │  │
│  └──────────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
         │                    │
    stdin/stdout         WebSocket + HTTP
    (agent/REPL)         (inspector UI)
```

### 2.1 Event Bus (`pkg/events`)

The runtime emits events for everything that happens. The inspector
subscribes to these events and streams them to the frontend.

```go
// pkg/events/events.go
type EventType string

const (
    EventCommandStart    EventType = "command.start"
    EventCommandEnd      EventType = "command.end"
    EventCommandError    EventType = "command.error"
    EventPipelineStart   EventType = "pipeline.start"
    EventPipelineEnd     EventType = "pipeline.end"
    EventPipelineStep    EventType = "pipeline.step"
    EventVerifyStart     EventType = "verify.start"
    EventVerifyResult    EventType = "verify.result"
    EventCheckpointSave  EventType = "checkpoint.save"
    EventCheckpointRestore EventType = "checkpoint.restore"
    EventContextChange   EventType = "context.change"
    EventPlanGenerated   EventType = "plan.generated"
    EventPlanApproval    EventType = "plan.approval_requested"
    EventPlanApproved    EventType = "plan.approved"
    EventPlanRejected    EventType = "plan.rejected"
    EventSpecLoaded      EventType = "spec.loaded"
    EventAgentMessage    EventType = "agent.message"   // raw LLM ↔ agsh messages
)

type Event struct {
    Type      EventType      `json:"type"`
    Timestamp time.Time      `json:"timestamp"`
    Data      any            `json:"data"`
    StepIndex int            `json:"step_index,omitempty"`  // pipeline step number
    Duration  time.Duration  `json:"duration,omitempty"`
}

type EventBus interface {
    Publish(event Event)
    Subscribe(filter ...EventType) <-chan Event
    History(since time.Time) []Event
}
```

### 2.2 Inspector HTTP Server (`internal/inspector`)

```go
// internal/inspector/server.go
type InspectorServer struct {
    eventBus    events.EventBus
    contextStore context.ContextStore
    checkpoints  verify.CheckpointManager
    registry     platform.Registry
    router       *http.ServeMux
}

func (s *InspectorServer) Start(port int) error {
    // Static assets (embedded)
    s.router.Handle("/", http.FileServer(http.FS(embeddedUI)))
    
    // WebSocket for live events
    s.router.HandleFunc("/ws", s.handleWebSocket)
    
    // REST endpoints for on-demand queries
    s.router.HandleFunc("/api/status", s.handleStatus)
    s.router.HandleFunc("/api/context", s.handleContext)
    s.router.HandleFunc("/api/history", s.handleHistory)
    s.router.HandleFunc("/api/checkpoints", s.handleCheckpoints)
    s.router.HandleFunc("/api/commands", s.handleCommands)
    s.router.HandleFunc("/api/plan", s.handlePlan)
    s.router.HandleFunc("/api/envelope/{id}", s.handleEnvelope)
    
    // Intervention endpoints
    s.router.HandleFunc("/api/approve", s.handleApprove)
    s.router.HandleFunc("/api/reject", s.handleReject)
    s.router.HandleFunc("/api/pause", s.handlePause)
    s.router.HandleFunc("/api/resume", s.handleResume)
    
    return http.ListenAndServe(fmt.Sprintf(":%d", port), s.router)
}
```

---

## 3. Frontend Views

The inspector UI is a single-page app with a sidebar nav and a main content
area. Five primary views, each answering a core question.

### 3.1 Dashboard (Home)

**Question:** What's happening right now?

```
┌──────────────────────────────────────────────────────────┐
│  agsh Inspector                          ● Live    ⏸ ▶  │
├──────────┬───────────────────────────────────────────────┤
│          │                                               │
│ ▶ Dash   │  Current Task: github-weekly-report           │
│   Plan   │  Status: ● Executing (Step 3 of 5)           │
│   Stream │  Elapsed: 12.4s                               │
│   Context│  ┌─────────────────────────────────────────┐  │
│   History│  │  ███████████░░░░░░░░░  Step 3/5  60%    │  │
│          │  └─────────────────────────────────────────┘  │
│          │                                               │
│          │  ┌─ Current Step ─────────────────────────┐   │
│          │  │ github:pr:list                          │   │
│          │  │ Intent: "Get PRs for last 7 days"      │   │
│          │  │ Status: ● Running (2.1s)               │   │
│          │  │ Input:  {repo: "cgast/agsh", ...}      │   │
│          │  └────────────────────────────────────────┘   │
│          │                                               │
│          │  ┌─ Quick Stats ──────────────────────────┐   │
│          │  │ Commands run: 4    Checkpoints: 1      │   │
│          │  │ Assertions:  6/6 ✓  Errors: 0          │   │
│          │  │ Context keys: 12   Envelopes: 4        │   │
│          │  └────────────────────────────────────────┘   │
│          │                                               │
└──────────┴───────────────────────────────────────────────┘
```

**Elements:**
- Task name and status (from loaded spec)
- Progress bar showing pipeline step progress
- Current step detail card (command, intent, timing, input preview)
- Quick stats (commands run, assertions passed/failed, errors, context size)
- Live/paused indicator with pause/resume controls

### 3.2 Plan View

**Question:** What does the agent intend to do, and can I change it?

```
┌──────────────────────────────────────────────────────────┐
│  Plan: github-weekly-report          [Approve] [Reject]  │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Goal: Generate a weekly summary of GitHub activity...   │
│  Risk Summary: 4 read-only API calls, 1 local file write│
│  Estimated: ~20s                                         │
│                                                          │
│  ┌─ Step 1 ────────────────── read-only ─── ✓ Done ──┐  │
│  │  github:repo:list                                   │  │
│  │  "Get all owned repos"                              │  │
│  │  → 12 repos returned (2.3s)                         │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ Step 2 ────────────────── read-only ─── ✓ Done ──┐  │
│  │  github:pr:list (×12 repos)                         │  │
│  │  "Get PRs from last 7 days per repo"                │  │
│  │  → 34 PRs found (4.1s)                              │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ Step 3 ────────────────── read-only ── ● Running ─┐  │
│  │  github:issue:list (×12 repos)                      │  │
│  │  "Get issues from last 7 days per repo"             │  │
│  │  → ... (1.8s elapsed)                               │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ Step 4 ──────────────────── write ──── ○ Pending ─┐  │
│  │  🔒 checkpoint.save "pre-write"                     │  │
│  │  fs:write → ./reports/weekly-2025-02-09.md          │  │
│  │  "Write the compiled report"                        │  │
│  │  Verify: not_empty, contains "##", llm_judge        │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Elements:**
- Goal and risk summary from the spec/plan
- Step cards with status indicators: ✓ Done, ● Running, ○ Pending, ✗ Failed
- Risk level per step (read-only, write, destructive) with color coding
- Checkpoint indicators (🔒)
- Verification requirements listed per step
- Approve/Reject buttons when a plan is awaiting approval
- Click any step to expand full input/output/envelope detail

### 3.3 Event Stream

**Question:** What happened, in real time?

```
┌──────────────────────────────────────────────────────────┐
│  Event Stream                    [Filter ▾] [Auto-scroll]│
├──────────────────────────────────────────────────────────┤
│                                                          │
│  14:23:01.003  spec.loaded                               │
│               github-weekly-report loaded (5 constraints)│
│                                                          │
│  14:23:01.015  plan.generated                            │
│               4 steps, risk: low, est: ~20s              │
│                                                          │
│  14:23:01.020  plan.approval_requested                   │
│               ⏳ Waiting for human approval...           │
│                                                          │
│  14:23:05.102  plan.approved                             │
│               Approved via inspector UI                   │
│                                                          │
│  14:23:05.110  pipeline.start                            │
│               Starting 4-step pipeline                    │
│                                                          │
│  14:23:05.112  command.start                             │
│               [1/4] github:repo:list {affiliation:owner} │
│                                                          │
│  14:23:07.445  command.end                               │
│               [1/4] ✓ 12 repos (2.3s)                   │
│                                                          │
│  14:23:07.450  verify.result                             │
│               [1/4] not_empty: ✓                         │
│                                                          │
│  14:23:07.452  command.start                             │
│               [2/4] github:pr:list (×12)                 │
│                                                          │
│  14:23:11.560  command.end                               │
│               [2/4] ✓ 34 PRs (4.1s)                     │
│                                                          │
│  14:23:11.565  command.start                             │
│  ●            [3/4] github:issue:list (×12)              │
│               ...                                        │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Elements:**
- Chronological event log with timestamps
- Color-coded event types (green for success, yellow for warnings, red for errors)
- Filter dropdown: All, Commands, Verification, Context, Agent Messages
- Click any event to expand full payload/envelope
- Auto-scroll toggle (follows latest events)
- Agent messages (raw LLM reasoning) shown inline when available

### 3.4 Context Explorer

**Question:** What data is the agent working with?

```
┌──────────────────────────────────────────────────────────┐
│  Context Explorer                         [Search 🔍]    │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Scope: [project] [session] [step] [history]             │
│                                                          │
│  ── project ─────────────────────────────────────────    │
│  │                                                       │
│  │  goal          "Generate a weekly summary..."  str    │
│  │  constraints   [5 items]                       []str  │
│  │  guidelines    [6 items]                       []str  │
│  │  params        {date_range_days: 7, ...}       map    │
│  │                                                       │
│  ── session ─────────────────────────────────────────    │
│  │                                                       │
│  │  repos         [{name: "agsh", ...}, ...]      []obj  │
│  │  ▶ repos[0]    {name: "agsh", stars: 42,       obj   │
│  │                  open_issues: 3, ...}                  │
│  │  prs           [{title: "Fix pipeline", ...}]  []obj  │
│  │  issues        [{title: "Bug: ...", ...}]      []obj  │
│  │  step_count    3                                int   │
│  │  last_command  "github:issue:list"              str   │
│  │                                                       │
│  ── step (current) ──────────────────────────────────    │
│  │                                                       │
│  │  command       "github:issue:list"              str   │
│  │  input_repo    "cgast/agsh"                     str   │
│  │  attempt       1                                int   │
│  │                                                       │
│  ────────────────────────────────────────────────────    │
│                                                          │
│  ┌─ Envelope Inspector ──────────────────────────────┐   │
│  │ Click any value above to inspect the full envelope │   │
│  │ including payload, metadata, and provenance chain  │   │
│  └────────────────────────────────────────────────────┘   │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Elements:**
- Scope tabs: project (from spec), session (runtime state), step (current), history
- Tree view of key-value pairs with type annotations
- Expandable nested objects/arrays
- Click any value to see the full envelope (payload + metadata + provenance)
- Search across all scopes
- Diff view: compare context state between two checkpoints

### 3.5 History & Checkpoints

**Question:** What happened in past runs, and can I go back?

```
┌──────────────────────────────────────────────────────────┐
│  History & Checkpoints                                   │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ── Checkpoints ─────────────────────────────────────    │
│                                                          │
│  🔒 pre-write         14:23:13  12 context keys  [Diff] │
│     "Before writing report file"                         │
│     [Restore]                                            │
│                                                          │
│  🔒 pre-execution     14:23:05   4 context keys  [Diff] │
│     "Before starting pipeline"                           │
│     [Restore]                                            │
│                                                          │
│  ── Execution History ───────────────────────────────    │
│                                                          │
│  Run #3 — github-weekly-report     14:23:01  ✓ Success   │
│    5 steps, 4 commands, 8/8 assertions passed            │
│    Duration: 18.2s                                       │
│    [View Details] [View Output]                          │
│                                                          │
│  Run #2 — csv-to-table             13:45:12  ✗ Failed    │
│    3 steps, 2 commands, 5/7 assertions passed            │
│    Failed: matches_regex on "Name column"                │
│    Rolled back to: pre-write                             │
│    [View Details] [View Error]                           │
│                                                          │
│  Run #1 — heading-counter          13:30:05  ✓ Success   │
│    3 steps, 3 commands, 1/1 assertions passed            │
│    Duration: 1.8s                                        │
│    [View Details] [View Output]                          │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Elements:**
- Checkpoint list with timestamps, context size, and restore/diff actions
- Diff view: side-by-side comparison of context state at two checkpoints
- Execution history: all past runs with status, step count, assertion results
- Click any run to see the full event stream replay
- Failed runs show the specific assertion that failed and what action was taken
- Link to view output files from successful runs

---

## 4. Implementation Approach

### 4.1 Backend (`internal/inspector/`)

```
internal/inspector/
├── server.go          # HTTP server setup, routing
├── websocket.go       # WebSocket handler for live events
├── handlers.go        # REST API handlers
├── middleware.go       # CORS, logging
└── ui/                # Embedded frontend assets
    ├── index.html
    ├── app.js         # Single JS file, no build step
    └── style.css
```

The frontend is deliberately simple — vanilla HTML/CSS/JS with no framework.
This keeps the binary small (embedded via `go:embed`) and avoids a separate
build step. For the prototype, this is sufficient. A future version could
use a lightweight framework if complexity grows.

### 4.2 Key Technical Decisions

**WebSocket for live updates.** The event bus publishes events, the inspector
server fans them out to all connected WebSocket clients. This gives real-time
updates without polling.

**Embedded UI via `go:embed`.** The frontend assets are compiled into the Go
binary. No separate file server, no CDN, no npm. One binary serves everything.

```go
//go:embed ui/*
var embeddedUI embed.FS
```

**REST for on-demand queries.** The WebSocket handles live streaming. REST
endpoints handle point-in-time queries (full context dump, checkpoint list,
command registry). This avoids overloading the WebSocket with request-response
patterns.

**No authentication for prototype.** The inspector binds to localhost only.
Future versions could add a token for remote access.

### 4.3 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ws` | WS | Live event stream |
| `/api/status` | GET | Current runtime status (task, step, timing) |
| `/api/context` | GET | Full context store dump (optional scope filter) |
| `/api/context/{scope}/{key}` | GET | Single context value with envelope |
| `/api/history` | GET | Execution history (paginated) |
| `/api/history/{run_id}` | GET | Full event log for a specific run |
| `/api/checkpoints` | GET | List all checkpoints |
| `/api/checkpoints/{name}/diff/{other}` | GET | Diff two checkpoints |
| `/api/commands` | GET | Command registry (names, schemas) |
| `/api/plan` | GET | Current plan (if any) |
| `/api/envelope/{id}` | GET | Full envelope by ID |
| `/api/approve` | POST | Approve pending plan |
| `/api/reject` | POST | Reject pending plan (with optional feedback) |
| `/api/pause` | POST | Pause pipeline execution |
| `/api/resume` | POST | Resume pipeline execution |

### 4.4 WebSocket Event Format

```json
{
    "type": "command.end",
    "timestamp": "2025-02-09T14:23:07.445Z",
    "data": {
        "command": "github:repo:list",
        "duration_ms": 2333,
        "status": "ok",
        "output_summary": "12 repos returned",
        "envelope_id": "env-a3f2c1"
    },
    "step_index": 1
}
```

The frontend filters and routes events to the appropriate view component
based on `type`.

---

## 5. Project Structure Addition

The inspector adds the following to the agsh project:

```
agsh/
├── ...
├── internal/
│   ├── inspector/           # Inspector GUI server
│   │   ├── server.go        # HTTP server, routing, lifecycle
│   │   ├── websocket.go     # WebSocket handler + event fan-out
│   │   ├── handlers.go      # REST API handlers
│   │   ├── middleware.go     # CORS, logging, localhost-only
│   │   └── ui/              # Embedded frontend (no build step)
│   │       ├── index.html   # Single-page app shell
│   │       ├── app.js       # All frontend logic
│   │       └── style.css    # Minimal, responsive styles
│   └── ...
│
├── pkg/
│   ├── events/              # Event bus (used by runtime + inspector)
│   │   ├── bus.go           # EventBus interface + in-memory impl
│   │   ├── types.go         # Event types and Event struct
│   │   └── bus_test.go
│   └── ...
```

### 5.1 Integration with agsh Binary

```go
// cmd/agsh/main.go
func main() {
    // ... existing mode detection ...
    
    if cfg.Inspector.Enabled {
        inspector := inspector.New(eventBus, contextStore, checkpoints, registry)
        go inspector.Start(cfg.Inspector.Port)
        fmt.Fprintf(os.Stderr, "Inspector running at http://localhost:%d\n", cfg.Inspector.Port)
    }
}
```

### 5.2 Configuration

```yaml
# .agsh/config.yaml addition
inspector:
  enabled: true
  port: 4200
  # bind: "127.0.0.1"    # localhost only (default)
  # bind: "0.0.0.0"      # all interfaces (use with caution)
```

### 5.3 CLI Flags

```bash
agsh --inspector                  # enable on default port 4200
agsh --inspector-port=8080        # enable on custom port
agsh --no-inspector               # explicitly disable
```

---

## 6. Build Integration

The inspector is part of Phase 4 (Agent Mode) or can be a parallel track
since it only depends on the Event Bus, which should be wired in from Phase 1.

### Suggested Build Order

1. **Phase 1 addition:** Implement `pkg/events` (EventBus) and wire it into
   the pipeline runtime. Every command execution emits events.
2. **Phase 2 addition:** Emit events for platform commands, spec loading,
   plan generation.
3. **Phase 3 addition:** Emit events for verification results, checkpoint
   operations.
4. **Phase 4 / parallel:** Build the inspector server and frontend.

### Milestone

The inspector is "done" when:
- `agsh --inspector` starts a web server alongside the REPL/agent mode
- Opening `http://localhost:4200` shows the dashboard with live data
- Running a demo spec shows real-time progress in the Plan View
- The Event Stream shows all events as they happen
- Context Explorer lets you browse the full context tree
- Approving/rejecting a plan from the inspector works
- History shows past runs with pass/fail details
