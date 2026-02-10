# First Steps with agsh

> Get agsh running, connect an agent, and observe what it does.

This guide walks you through installing agsh, running your first pipeline,
connecting an AI agent, and using the inspector to watch it work.

---

## Prerequisites

- **Go 1.22+** installed ([download](https://go.dev/dl/))
- **Docker** and **Docker Compose** (optional, for sandboxed runs)
- **Git** for cloning the repository

---

## 1. Build agsh

Clone the repository and build the binary:

```bash
git clone https://github.com/cgast/agsh.git
cd agsh
make build
```

This produces the `bin/agsh` binary. Verify it works:

```bash
./bin/agsh
```

You should see:

```
agsh v0.1.0 — Agent Shell
Type 'help' for available commands, 'exit' to quit.

agsh>
```

Type `exit` to leave the REPL for now.

### Alternative: Docker

If you prefer to run in a sandboxed container:

```bash
make docker-build
docker-compose -f docker/docker-compose.yaml up
```

---

## 2. Explore the Interactive Shell

Start the REPL and try some commands:

```bash
./bin/agsh
```

### List available commands

```
agsh> commands
  fs:list              [fs] List files and directories
  fs:read              [fs] Read file contents
  fs:write             [fs] Write content to a file
```

These are **platform commands** — the building blocks agents use to interact
with the system. Each command takes an envelope in and produces an envelope out.

### Run a command

List files in a directory:

```
agsh> fs:list ./examples/demo/01-basic-pipeline/workspace
```

Read a file:

```
agsh> fs:read ./examples/demo/01-basic-pipeline/workspace/project-alpha.md
```

### Pipe commands together

Commands compose with `|`, passing envelopes between them:

```
agsh> fs:list ./examples/demo/01-basic-pipeline/workspace | fs:read
```

### Inspect the context store

agsh maintains a shared context store that commands can read from and write to.
This is how state flows between pipeline steps — not just through pipes, but
through named keys scoped to the project, session, or individual step.

```
agsh> context set session greeting "hello world"
OK
agsh> context get session greeting
hello world
agsh> context list session
  greeting = hello world
```

---

## 3. Run Your First Demo

agsh includes runnable demo scenarios. Demo 01 is a self-contained pipeline
that counts markdown headings across files — no external APIs required.

```bash
./bin/agsh demo 01
```

This runs the heading-counter pipeline from
`examples/demo/01-basic-pipeline/`. You will see event output on stderr and the
final summary on stdout:

```
=== Demo 01: Heading Counter ===
Workspace: ./examples/demo/01-basic-pipeline/workspace
Found 3 markdown files
  notes.md: 2 headings
  project-alpha.md: 3 headings
  project-beta.md: 5 headings

=== Output ===
# Heading Summary

| File | Headings |
|------|----------|
| notes.md | 2 |
| project-alpha.md | 3 |
| project-beta.md | 5 |

**Total: 10 headings across 3 files**

Written to: ./examples/demo/01-basic-pipeline/output.md
=== Demo 01 Complete ===
```

You can also specify a custom workspace and output path:

```bash
./bin/agsh demo 01 ./my-markdown-dir ./my-output.md
```

### What happened

The demo executed three pipeline steps:

1. **`fs:list`** — listed all files in the workspace directory
2. **`fs:read`** — read each `.md` file and counted lines starting with `#`
3. **`fs:write`** — wrote the summary table to `output.md`

Each step produced an envelope carrying structured data and metadata. The
context store accumulated per-file heading counts along the way. Events were
published at each step for observability.

---

## 4. Understand Project Specs

Agents don't improvise from scratch. A human writes a **project spec** — a YAML
file that declares *what* to do, and the agent figures out *how*.

Here is the spec for Demo 01 (`examples/demo/01-basic-pipeline/project.agsh.yaml`):

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "heading-counter"
  description: "Count markdown headings across files and produce a summary"

goal: |
  Scan all markdown files in the workspace directory, extract headings
  (lines starting with #), count them per file, and write a summary
  report to output.md.

constraints:
  - "Read-only access to workspace files (only write output.md)"
  - "Must complete within 10 seconds"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Summary must not be empty"

allowed_commands:
  - "fs:*"

output:
  path: "./output.md"
  format: "markdown"
```

Key sections:

- **`goal`** — Natural language description of what to accomplish
- **`constraints`** — Boundaries the agent must respect
- **`allowed_commands`** — Which platform commands the agent may use (glob patterns supported)
- **`success_criteria`** — Assertions the runtime checks after execution
- **`output`** — Where and in what format to write results

The spec is the contract between human intent and agent execution. The human
says *what*; the agent plans *how*; the runtime *verifies* the result.

---

## 5. Connect an Agent

agsh is designed to be driven by an LLM over a structured protocol. In agent
mode, agsh communicates via JSON-RPC over stdin/stdout — the same interface an
LLM orchestrator uses to send commands and receive results.

> **Note:** Agent mode is under active development (Phase 4). The protocol and
> workflow described here reflect the target design. Check the
> [architecture docs](architecture.md) for current implementation status.

### Start in agent mode

```bash
./bin/agsh --mode=agent
```

Or via environment variable:

```bash
AGSH_MODE=agent ./bin/agsh
```

Or in Docker (the default compose config already sets agent mode):

```bash
docker-compose -f docker/docker-compose.yaml up
```

### The agent protocol

The agent communicates by sending JSON-RPC messages to agsh's stdin and reading
responses from stdout. A typical session looks like this:

```
Agent                                    agsh
  │                                        │
  │─── commands.list ─────────────────────>│
  │<── [fs:list, fs:read, fs:write, ...]  │
  │                                        │
  │─── project.load {spec: "...yaml"} ───>│
  │<── {goal, constraints, criteria}       │
  │                                        │
  │─── project.plan ──────────────────────>│
  │<── {steps: [...], risk: "low"}         │
  │                                        │
  │─── project.approve ──────────────────>│
  │<── {status: "approved"}                │
  │                                        │
  │─── execute {command: "fs:list"} ──────>│
  │<── {payload: [...files...]}            │
  │                                        │
  │─── checkpoint.save {name: "pre-write"}>│
  │<── {id: "chk-001"}                     │
  │                                        │
  │─── execute {command: "fs:write"} ─────>│
  │<── {payload: {bytes_written: 342}}     │
```

### Agent workflow

A typical agent session follows this pattern:

1. **Discover** — the agent calls `commands.list` to learn what commands are
   available
2. **Load** — the agent loads a project spec with `project.load`
3. **Plan** — the agent generates an execution plan with `project.plan`
4. **Approve** — depending on the approval mode, the plan may require human
   approval before execution proceeds
5. **Execute** — the agent runs commands one at a time, piping envelopes
   between steps
6. **Checkpoint** — before destructive operations, the agent saves a checkpoint
   so it can roll back on failure
7. **Verify** — after execution, the runtime checks success criteria from the
   spec

### Writing an orchestrator

To connect your own LLM, you write a thin orchestrator script that:

1. Starts agsh in agent mode as a subprocess
2. Sends JSON-RPC messages to its stdin
3. Reads JSON-RPC responses from its stdout
4. Passes the responses to the LLM and relays the LLM's decisions back

Here is a minimal example in Python:

```python
import subprocess
import json

# Start agsh as a subprocess in agent mode.
proc = subprocess.Popen(
    ["./bin/agsh", "--mode=agent"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True,
)

def send(method, params=None):
    """Send a JSON-RPC request and read the response."""
    msg = {"jsonrpc": "2.0", "method": method, "id": 1}
    if params:
        msg["params"] = params
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()
    line = proc.stdout.readline()
    return json.loads(line)

# 1. Discover available commands.
commands = send("commands.list")

# 2. Load a project spec.
spec = send("project.load", {"spec": "examples/demo/01-basic-pipeline/project.agsh.yaml"})

# 3. Execute a command.
result = send("execute", {"command": "fs:list", "args": {"path": "./workspace"}})

# 4. Feed the result to your LLM, get next action, repeat.
```

In a real orchestrator, steps 3-4 loop: the LLM sees each result, decides the
next command, and the orchestrator relays it to agsh until the task is complete.

---

## 6. Watch the Agent

When an agent is running autonomously, you need visibility into what it is
doing. agsh provides several ways to observe execution.

### Terminal event stream

When running demos or pipelines, agsh emits events to stderr. These show
each command starting, completing, and any errors:

```
[event] pipeline.start: map[demo:01-heading-counter step_count:3]
[event] command.start: map[command:fs:list]
[event] command.end: map[command:fs:list status:ok]
[event] command.start: map[command:fs:read intent:Read each markdown file and count headings]
[event] command.end: map[command:fs:read files_read:3 status:ok]
[event] command.start: map[command:fs:write]
[event] command.end: map[command:fs:write status:ok]
[event] pipeline.end: map[success:true]
```

You can watch these events in a separate terminal while the agent runs:

```bash
# Run agsh, redirect stdout to a file, watch events on stderr.
./bin/agsh demo 01 2>&1 1>/dev/null
```

### The inspector GUI

> **Note:** The inspector is under active development (Phase 4). The
> description below reflects the target design. See the full spec in
> [inspector-gui.md](inspector-gui.md).

The inspector is a web-based UI embedded in the agsh binary. Start it with:

```bash
./bin/agsh --inspector
# or with a custom port:
./bin/agsh --inspector-port=4200
```

Then open `http://localhost:4200` in your browser. The inspector provides
five views:

**Dashboard** — At-a-glance status: current task, progress bar, step timing,
and quick stats (commands run, errors, context entries).

**Plan View** — The agent's intended execution steps displayed as a list. Each
step shows the command name, risk level (read-only vs. write), and whether
verification is required. You can approve or reject the plan from here.

**Event Stream** — A live chronological log of everything happening inside
agsh. Events are color-coded by type: blue for commands, green for
verification, yellow for context changes, red for errors.

**Context Explorer** — A tree view of the context store, organized by scope
(project, session, step, history). You can see what data the agent is
accumulating and how it changes over time.

**Checkpoints** — A list of saved states the agent can roll back to if
something goes wrong. Each checkpoint shows when it was taken and what the
context looked like at that point.

### Approval flow

When the approval mode is set to `plan` (the default in `.agsh/config.yaml`),
the agent must present its plan before executing. You can review and
approve/reject from either the inspector UI or the terminal.

The approval modes are:

| Mode          | Behavior                                       |
|---------------|------------------------------------------------|
| `always`      | Every command requires approval                |
| `plan`        | Approve the plan once, then execution proceeds |
| `destructive` | Only write/delete operations need approval     |
| `never`       | No approval required (fully autonomous)        |

Set the mode in `.agsh/config.yaml`:

```yaml
approval:
  mode: plan        # always | plan | destructive | never
  timeout: 300      # seconds before auto-reject
```

---

## 7. What's Next

Now that you have agsh running and understand the basics, here are some things
to explore:

- **Read the [architecture docs](architecture.md)** for the full system design,
  including the three pillars (context, platforms, verification) and how they
  compose.
- **Study the [demo specs](demo-specs.md)** to see four progressively complex
  scenarios, from basic pipelines to full agent autonomy with failure recovery.
- **Read the [inspector spec](inspector-gui.md)** for the full design of the
  observation UI.
- **Write your own spec** — create a `project.agsh.yaml` for a task you care
  about. Define the goal, constraints, and success criteria. Even before full
  spec loading is implemented, this is a useful exercise for thinking about
  the human-agent contract.
- **Experiment in the REPL** — try composing `fs:list`, `fs:read`, and
  `fs:write` in different pipeline combinations. Store intermediate results
  in the context store and retrieve them in later commands.

### Current implementation status

agsh is in early development. Here is what works today and what is coming:

| Feature                 | Status       |
|-------------------------|-------------|
| Interactive REPL        | Working     |
| Filesystem commands     | Working     |
| Pipeline execution      | Working     |
| Context store           | Working     |
| Event bus               | Working     |
| Demo 01                 | Working     |
| GitHub/HTTP commands    | Planned     |
| Spec loading            | Planned     |
| Verification engine     | Planned     |
| Agent mode (JSON-RPC)   | Planned     |
| Inspector GUI           | Planned     |

Check `CLAUDE.md` in the project root for the detailed build phase checklist.
