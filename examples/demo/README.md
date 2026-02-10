# agsh Demo Scenarios

Four runnable demos that showcase agsh capabilities. Each is self-contained
in its own directory with a `project.agsh.yaml` spec and workspace files.

## Quick Run

```bash
# Build agsh first
go build -o bin/agsh ./cmd/agsh

# Demo 01: Basic Pipeline (no external deps)
./bin/agsh demo 01

# Demo 02: GitHub Report (uses mock data, no token required)
./bin/agsh demo 02

# Demo 03: Verified Transform (CSV -> markdown with assertions)
./bin/agsh demo 03

# Demo 04: Agent Autonomy (full JSON-RPC agent loop)
./bin/agsh demo 04
```

## Demos

### 01 — Basic Pipeline

Reads markdown files from a workspace, counts headings per file, and writes
a summary table. Tests envelope flow, context store, and fs commands.

**Directory:** `01-basic-pipeline/`

### 02 — GitHub Report

Generates a weekly GitHub activity report. Demonstrates spec loading, plan
generation, protocol handler flow, and verification. Uses mock data when
no `GITHUB_TOKEN` is available.

**Directory:** `02-github-report/`

### 03 — Verified File Transform

Transforms a CSV file into a formatted markdown table with full verification.
Tests all assertion types (`not_empty`, `contains`, `count_gte`, `matches_regex`,
`not_contains`, `llm_judge`), checkpoint save/restore, and rollback on failure.

Run with bad input to test failure handling:
```bash
./bin/agsh demo 03 ./examples/demo/03-verified-transform/workspace/team-bad.csv
```

**Directory:** `03-verified-transform/`

### 04 — Agent Autonomy

Full end-to-end agent loop via JSON-RPC protocol handler. Simulates what an
LLM orchestrator would do: discover commands, load spec, generate plan,
checkpoint, execute steps, verify, and write output.

**Directory:** `04-agent-autonomy/`

## Validation Checklist

| Demo | Tests | Pass Criteria |
|------|-------|---------------|
| 01 | Envelope flow, context, fs commands | Output matches expected markdown |
| 02 | Platform commands, spec loading, plan generation | Report contains GitHub data with links |
| 03 (success) | All assertion types, transform quality | All 7 assertions pass |
| 03 (failure) | Checkpoint, rollback, error reporting | Failure detected, state rolled back |
| 04 | Full agent loop, recovery, autonomy | Agent completes task, report passes criteria |
