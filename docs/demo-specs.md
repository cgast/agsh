# agsh Demo Specs

> Runnable demo scenarios for testing and showcasing agsh capabilities.
> Each demo is self-contained in `examples/demo/NN-name/` with a
> `project.agsh.yaml` and any required workspace files.

---

## Demo 01: Basic Pipeline

**Purpose:** Validate that envelopes, context, and pipelines work end-to-end
with only local filesystem commands. No external APIs, no verification — just
the data flow.

**What it does:** Takes a directory of markdown files, extracts all headings,
counts them per file, and writes a summary.

### Spec: `examples/demo/01-basic-pipeline/project.agsh.yaml`

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "heading-counter"
  description: "Count markdown headings across files and produce a summary"
  author: "demo"
  created: "2025-02-09"
  tags: ["demo", "basic", "filesystem"]

goal: |
  Scan all markdown files in the workspace directory, extract headings
  (lines starting with #), count them per file, and write a summary
  report to output.md.

constraints:
  - "Read-only access to workspace files (only write output.md)"
  - "Must complete within 10 seconds"

guidelines:
  - "Summary should list each file with its heading count"
  - "Sort files alphabetically"
  - "Include total heading count at the bottom"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Summary must not be empty"

allowed_commands:
  - "fs:*"

output:
  path: "./output.md"
  format: "markdown"

params: []
```

### Workspace: `examples/demo/01-basic-pipeline/workspace/`

```
workspace/
├── project-alpha.md      # 3 headings
├── project-beta.md       # 5 headings
└── notes.md              # 2 headings
```

**`project-alpha.md`:**
```markdown
# Project Alpha

## Overview
Alpha is a demo project for testing agsh pipelines.

## Goals
- Validate envelope flow
- Test context passing

## Status
In progress.
```

**`project-beta.md`:**
```markdown
# Project Beta

## Architecture
Microservices-based design.

## Components
### Frontend
React application.

### Backend
Go API server.

## Deployment
Docker-based.
```

**`notes.md`:**
```markdown
# Meeting Notes

## Action Items
- Review pipeline design
- Write more tests
```

### Expected Output

```markdown
# Heading Summary

| File | Headings |
|------|----------|
| notes.md | 2 |
| project-alpha.md | 3 |
| project-beta.md | 5 |

**Total: 10 headings across 3 files**
```

### What This Tests

- Envelope creation from `fs:list`
- Envelope passing through pipeline steps
- Context store (accumulating counts across iterations)
- `fs:write` producing final output
- Basic `not_empty` assertion

---

## Demo 02: GitHub Report

**Purpose:** Validate platform commands by hitting a real external API.
Demonstrates namespace resolution, credential loading, and structured
data flowing through pipelines.

**What it does:** Fetches GitHub activity for a given user's repos over the
last 7 days and produces a markdown report.

### Spec: `examples/demo/02-github-report/project.agsh.yaml`

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "github-weekly-report"
  description: "Weekly summary of GitHub activity across owned repos"
  author: "demo"
  created: "2025-02-09"
  tags: ["demo", "github", "platform", "reporting"]

goal: |
  Generate a markdown report summarizing GitHub activity for the
  authenticated user's repositories over the past N days. Include
  PRs (opened, merged, closed), issues (opened, closed), and
  notable commits.

constraints:
  - "Only include repos owned by the authenticated user"
  - "Do not create, modify, or delete any GitHub resources (read-only)"
  - "Output must be a single markdown file"
  - "Must complete within 60 seconds"
  - "Respect GitHub API rate limits"

guidelines:
  - "Group activity by repository"
  - "Within each repo, group by type: PRs, Issues, Commits"
  - "Keep descriptions to 1-2 sentences per item"
  - "Include direct links to GitHub"
  - "Flag PRs that have been open longer than 7 days"
  - "If a repo had no activity, omit it from the report"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Report must not be empty"
  - type: "contains"
    target: "output"
    expected: "## "
    message: "Report must contain markdown section headers"
  - type: "contains"
    target: "output"
    expected: "github.com"
    message: "Report must contain GitHub links"
  - type: "llm_judge"
    target: "output"
    expected: "The report covers GitHub activity grouped by repository with PRs, issues, and commits"
    message: "Report structure must match the stated goal"

allowed_commands:
  - "github:repo:list"
  - "github:pr:list"
  - "github:issue:list"
  - "github:commit:list"
  - "fs:write"

output:
  path: "./reports/weekly-{{date}}.md"
  format: "markdown"

params:
  - name: "date_range_days"
    type: "integer"
    default: 7
    description: "How many days back to look"
  - name: "include_forks"
    type: "boolean"
    default: false
    description: "Include forked repos in the report"
```

### Expected Plan

When `agsh run` processes this spec, the generated plan should look like:

```json
{
    "spec": "github-weekly-report",
    "steps": [
        {
            "command": "github:repo:list",
            "args": {"affiliation": "owner", "type": "sources"},
            "intent": "Get all owned repos (excluding forks)",
            "risk": "read-only"
        },
        {
            "command": "github:pr:list",
            "args": {"state": "all", "since": "{{7_days_ago}}", "repo": "{{each_repo}}"},
            "intent": "Get PRs from the last 7 days per repo",
            "risk": "read-only",
            "note": "Iterates over repos from previous step"
        },
        {
            "command": "github:issue:list",
            "args": {"state": "all", "since": "{{7_days_ago}}", "repo": "{{each_repo}}"},
            "intent": "Get issues from the last 7 days per repo",
            "risk": "read-only"
        },
        {
            "command": "fs:write",
            "args": {"path": "./reports/weekly-2025-02-09.md"},
            "intent": "Write the compiled report",
            "risk": "write",
            "checkpoint_before": true
        }
    ],
    "estimated_duration": "~20s",
    "risk_summary": "Multiple read-only GitHub API calls, 1 local file write"
}
```

### What This Tests

- Platform command registry and namespace resolution (`github:*`)
- Credential loading from `.agsh/platforms.yaml`
- Structured JSON payloads flowing through envelopes
- Iteration pattern (one command per repo from a previous step's output)
- `llm_judge` assertion type
- Param interpolation (`date_range_days`)
- Template variables in output path (`{{date}}`)

### Setup Requirements

```bash
# .agsh/platforms.yaml must contain:
github:
  token: "${GITHUB_TOKEN}"
```

---

## Demo 03: Verified File Transform

**Purpose:** Showcase the verification engine. Deliberately includes a
transformation that can fail, demonstrating assertion checking, failure
handling, and checkpoint rollback.

**What it does:** Takes a CSV file, transforms it into a formatted markdown
table, verifies the output meets structural requirements, and rolls back
if verification fails.

### Spec: `examples/demo/03-verified-transform/project.agsh.yaml`

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "csv-to-table"
  description: "Transform CSV data into a verified markdown table"
  author: "demo"
  created: "2025-02-09"
  tags: ["demo", "verification", "transform", "checkpoint"]

goal: |
  Read a CSV file containing team member data, transform it into a
  properly formatted markdown table, verify the output meets all
  structural requirements, and write the result.

constraints:
  - "Preserve all data from the original CSV — no rows may be lost"
  - "Output must be valid markdown"
  - "Column order must match the CSV header order"

guidelines:
  - "Add a title header before the table"
  - "Right-align numeric columns"
  - "Sort rows by the first column (name) alphabetically"
  - "Add a summary row at the bottom with totals for numeric columns"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Output must not be empty"
  - type: "contains"
    target: "output"
    expected: "|"
    message: "Output must contain markdown table pipes"
  - type: "contains"
    target: "output"
    expected: "---"
    message: "Output must contain markdown table separator"
  - type: "count_gte"
    target: "output.lines"
    expected: 8
    message: "Output must have at least 8 lines (header + separator + 5 data rows + summary)"
  - type: "matches_regex"
    target: "output"
    expected: "\\| Name\\s*\\|"
    message: "Table must start with a Name column"
  - type: "not_contains"
    target: "output"
    expected: ",,,"
    message: "Output must not contain raw CSV artifacts"
  - type: "llm_judge"
    target: "output"
    expected: "A well-formatted markdown table with all 5 team members, sorted alphabetically, with a totals row"
    message: "Overall output quality check"

allowed_commands:
  - "fs:read"
  - "fs:write"

output:
  path: "./team-table.md"
  format: "markdown"

params:
  - name: "input_file"
    type: "string"
    default: "./workspace/team.csv"
    description: "Path to the input CSV file"
```

### Workspace: `examples/demo/03-verified-transform/workspace/`

**`team.csv`:**
```csv
Name,Role,Experience_Years,Projects_Completed
Charlie,Backend Engineer,5,23
Alice,Frontend Developer,3,12
Eve,DevOps Lead,7,31
Bob,Product Manager,4,18
Diana,Data Scientist,6,27
```

### Expected Output: `team-table.md`

```markdown
# Team Overview

| Name | Role | Experience (Years) | Projects Completed |
|------|------|-------------------:|-------------------:|
| Alice | Frontend Developer | 3 | 12 |
| Bob | Product Manager | 4 | 18 |
| Charlie | Backend Engineer | 5 | 23 |
| Diana | Data Scientist | 6 | 27 |
| Eve | DevOps Lead | 7 | 31 |
| **Total** | | **25** | **111** |
```

### Failure Scenario

To test rollback, include a second CSV that triggers a verification failure:

**`workspace/team-bad.csv`:**
```csv
Name,Role,Experience_Years,Projects_Completed
,Backend Engineer,5,23
Alice,,3,12
```

Running with `input_file: ./workspace/team-bad.csv` should:
1. Checkpoint before writing
2. Attempt the transform
3. Fail on `matches_regex` (empty Name field) or `llm_judge`
4. Roll back to the checkpoint
5. Report the failure with details

### What This Tests

- Multiple assertion types in combination
- `count_gte` on line count
- `matches_regex` for structural validation
- `not_contains` for negative assertions
- `llm_judge` for qualitative assessment
- Checkpoint creation before write operations
- Rollback on verification failure
- Meaningful error reporting with assertion details

---

## Demo 04: Agent Autonomy

**Purpose:** Full end-to-end agent loop. An LLM connects to agsh via
JSON-RPC, discovers commands, loads a spec, plans, executes, and
handles a mid-pipeline failure — all autonomously.

**What it does:** The agent is given a spec that requires multiple steps
with a deliberate failure point. It must discover available commands,
create a plan, execute with verification, encounter a failure, decide
how to recover, and complete the task.

### Spec: `examples/demo/04-agent-autonomy/project.agsh.yaml`

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "repo-health-check"
  description: "Analyze a GitHub repo and produce a health report with recommendations"
  author: "demo"
  created: "2025-02-09"
  tags: ["demo", "agent", "autonomy", "github", "analysis"]

goal: |
  Perform a health check on a given GitHub repository. Analyze its
  recent activity, open issues, PR velocity, and staleness indicators.
  Produce a health report with a score and actionable recommendations.

  The agent should:
  1. Discover what commands are available
  2. Gather data from multiple sources (repo info, PRs, issues)
  3. Analyze the data and compute health metrics
  4. Write a structured report
  5. Verify the report meets quality standards

constraints:
  - "Read-only access to GitHub"
  - "Must handle API failures gracefully (retry or skip with note)"
  - "Report must be factual — no invented data"
  - "Complete within 120 seconds"

guidelines:
  - "Health score should be 0-100 based on defined metrics"
  - "Metrics to consider: days since last commit, open PR age, issue response time, bus factor"
  - "Each recommendation should be specific and actionable"
  - "Include a summary section at the top"
  - "Use emoji indicators: 🟢 healthy, 🟡 warning, 🔴 critical"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Report must not be empty"
  - type: "contains"
    target: "output"
    expected: "Health Score"
    message: "Report must include a health score"
  - type: "contains"
    target: "output"
    expected: "Recommendation"
    message: "Report must include recommendations"
  - type: "matches_regex"
    target: "output"
    expected: "[🟢🟡🔴]"
    message: "Report must include health indicators"
  - type: "llm_judge"
    target: "output"
    expected: "A structured health report with a numeric score, multiple metrics with status indicators, and specific actionable recommendations"
    message: "Report must be comprehensive and well-structured"

allowed_commands:
  - "github:*"
  - "fs:write"
  - "http:get"

output:
  path: "./reports/health-{{repo_name}}.md"
  format: "markdown"

params:
  - name: "repo"
    type: "string"
    default: "golang/go"
    description: "GitHub repo in owner/name format"
  - name: "lookback_days"
    type: "integer"
    default: 30
    description: "Days of history to analyze"
```

### Orchestrator Script: `examples/demo/04-agent-autonomy/orchestrator.sh`

A shell script that connects an LLM to agsh for the demo:

```bash
#!/bin/bash
# orchestrator.sh — connects an LLM to agsh for autonomous operation
#
# Usage: ./orchestrator.sh [--model claude-sonnet-4-20250514] [--spec project.agsh.yaml]
#
# This script:
# 1. Starts agsh in agent mode (JSON-RPC over stdin/stdout)
# 2. Sends the spec to the LLM along with agsh's command catalog
# 3. Relays LLM decisions to agsh and agsh responses back to the LLM
# 4. Logs all interactions to ./logs/

MODEL="${MODEL:-claude-sonnet-4-20250514}"
SPEC="${SPEC:-project.agsh.yaml}"
LOG_DIR="./logs/$(date +%Y%m%d-%H%M%S)"

mkdir -p "$LOG_DIR"

echo "Starting agsh in agent mode..."
echo "Model: $MODEL"
echo "Spec: $SPEC"
echo "Logs: $LOG_DIR"

# System prompt for the LLM orchestrator
SYSTEM_PROMPT=$(cat <<'EOF'
You are an autonomous agent operating inside agsh (agent shell).
You communicate via JSON-RPC. Available methods:

- commands.list: Discover available commands
- commands.describe <name>: Get schema for a command
- project.load <spec>: Load a project spec
- project.plan: Generate an execution plan from the loaded spec
- execute <command> <args>: Run a single command
- pipeline <steps>: Run a multi-step pipeline
- context.get/set: Read/write shared context
- checkpoint.save/restore: Manage state checkpoints
- history: View execution log

Your workflow:
1. Load the project spec
2. Discover available commands with commands.list
3. Generate a plan with project.plan
4. Execute the plan step by step
5. If a step fails, decide: retry, skip, or rollback
6. Verify the final output against success criteria

Always checkpoint before destructive operations.
Always verify after each significant step.
Report your reasoning alongside each action.
EOF
)

# The actual orchestration loop would be implemented here.
# For the prototype, this serves as documentation of the intended flow.
echo "Orchestrator script is a reference implementation."
echo "See the system prompt above for the LLM's operating instructions."
echo ""
echo "To run manually:"
echo "  docker-compose exec agsh agsh --mode=agent < commands.jsonl"
```

### Expected Agent Behavior

The LLM should autonomously:

```
1. → commands.list
   ← [github:repo:info, github:pr:list, github:issue:list, fs:write, ...]

2. → project.load "project.agsh.yaml"
   ← {spec parsed, goal: "Perform a health check...", params: {repo: "golang/go"}}

3. → project.plan
   ← {steps: [...], risk_summary: "4 read-only API calls, 1 file write"}

4. → checkpoint.save "pre-execution"

5. → execute github:repo:info {repo: "golang/go"}
   ← {payload: {stars: 125000, forks: 17000, open_issues: 9200, ...}}

6. → execute github:pr:list {repo: "golang/go", state: "open", since: "30d"}
   ← {payload: [...], meta: {count: 342}}

7. → execute github:issue:list {repo: "golang/go", state: "open", since: "30d"}
   ← {payload: [...], meta: {count: 890}}
   
   [Agent computes health metrics from gathered data]

8. → checkpoint.save "pre-write"

9. → execute fs:write {path: "./reports/health-golang-go.md", content: "..."}
   ← {verification: {passed: true, results: [...]}}

10. Agent reports: "Task complete. Health report written. All verifications passed."
```

### Failure Injection

To test recovery, the demo environment can simulate failures:

```yaml
# In .agsh/config.yaml for this demo
debug:
  simulate_failures:
    - command: "github:pr:list"
      failure_rate: 0.5        # 50% chance of API timeout
      error: "API rate limit exceeded, retry after 60s"
```

Expected agent recovery:
1. Agent encounters the simulated failure
2. Checks the error type (rate limit → retryable)
3. Waits or uses cached data from context
4. Retries and succeeds
5. Notes the retry in the execution log

### What This Tests

- Full JSON-RPC agent mode protocol
- Command discovery (`commands.list`, `commands.describe`)
- Spec loading and plan generation
- Multi-step autonomous execution
- Mid-pipeline failure handling and recovery
- Checkpoint before risky operations
- Multiple verification types on final output
- `llm_judge` for qualitative assessment
- End-to-end orchestrator integration

---

## Running the Demos

### Prerequisites

```bash
# Build and start agsh
docker-compose up -d --build

# Set credentials (for demos 02 and 04)
export GITHUB_TOKEN="your-token-here"
```

### Quick Run

```bash
# Demo 01: Basic pipeline (no external deps)
docker-compose exec agsh agsh run /workspace/examples/demo/01-basic-pipeline/project.agsh.yaml

# Demo 02: GitHub report (requires GITHUB_TOKEN)
docker-compose exec agsh agsh run /workspace/examples/demo/02-github-report/project.agsh.yaml

# Demo 03: Verified transform (test success)
docker-compose exec agsh agsh run /workspace/examples/demo/03-verified-transform/project.agsh.yaml

# Demo 03: Verified transform (test failure + rollback)
docker-compose exec agsh agsh run /workspace/examples/demo/03-verified-transform/project.agsh.yaml \
  --param input_file=./workspace/team-bad.csv

# Demo 04: Agent autonomy (requires GITHUB_TOKEN + LLM API access)
cd examples/demo/04-agent-autonomy && ./orchestrator.sh
```

### Validation Checklist

| Demo | Tests | Pass Criteria |
|------|-------|---------------|
| 01 | Envelope flow, context, fs commands | Output matches expected markdown |
| 02 | Platform commands, credentials, iteration | Report contains grouped GitHub data with links |
| 03 (success) | All assertion types, transform quality | All 7 assertions pass |
| 03 (failure) | Checkpoint, rollback, error reporting | Failure detected, state rolled back, clear error message |
| 04 | Full agent loop, recovery, autonomy | Agent completes task with ≤2 retries, report passes all criteria |
