# First Steps with agsh

A beginner's guide to running your first AI agent with agsh, watching it work through the Inspector UI, and understanding the security model.

---

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose installed
- A terminal (bash, zsh, or similar)
- (Optional) A GitHub personal access token if you want to use GitHub commands

---

## 1. Build and Start agsh

Clone the repository and start the Docker environment:

```bash
git clone https://github.com/your-org/agsh.git
cd agsh

# Build the container
docker-compose -f docker/docker-compose.yaml up -d --build
```

This builds agsh from source inside a multi-stage Docker build and starts it in a sandboxed container. The container runs as a non-root user (`agsh`) with access limited to `/workspace`.

To verify the build succeeded:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh agsh --help
```

---

## 2. Run Your First Spec

agsh uses **spec files** (YAML) to define what an agent should do. A spec describes a goal, constraints, allowed commands, and success criteria. Start with the built-in basic demo:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh \
  agsh run examples/demo/01-basic-pipeline/project.agsh.yaml
```

This demo scans markdown files, counts headings, and writes a summary — all using only filesystem commands. It requires no external APIs or credentials.

**What happens:**
1. agsh loads the spec file
2. A plan is generated from the spec
3. You are shown the plan and asked to approve it
4. The pipeline executes step by step
5. Success criteria are verified against the output

---

## 3. Try Interactive Mode

You can also explore agsh interactively before running full specs:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh agsh
```

This drops you into the agsh REPL:

```
agsh> commands                     # list all available commands
agsh> fs:list /workspace           # list files in the workspace
agsh> fs:read /workspace/README.md # read a file
agsh> context list                 # view context scopes
agsh> help                         # show help
agsh> exit                         # quit
```

Interactive mode is useful for understanding what commands are available before writing your own specs.

---

## 4. Connect an LLM Agent

agsh is designed to be driven by an LLM. In **agent mode**, agsh reads JSON-RPC 2.0 requests from stdin and writes responses to stdout. This is how an external LLM orchestrator communicates with agsh.

### Start agent mode

```bash
docker-compose -f docker/docker-compose.yaml exec agsh agsh --mode=agent
```

### The agent protocol

The LLM sends JSON-RPC requests, and agsh responds. A typical agent session follows this lifecycle:

```
LLM                          agsh
 │                             │
 ├─ commands.list ────────────►│  "What can I do?"
 │◄──── list of commands ──────┤
 │                             │
 ├─ project.load ─────────────►│  "Here's what I want to achieve"
 │◄──── spec loaded ───────────┤
 │                             │
 ├─ project.plan ─────────────►│  "Make me a plan"
 │◄──── execution plan ────────┤
 │                             │
 ├─ project.approve ──────────►│  "Looks good, go ahead"
 │◄──── results + verification─┤
 │                             │
```

### Example: send a single command

```json
{"jsonrpc":"2.0","id":"1","method":"commands.list","params":{}}
```

agsh responds with all registered commands, their descriptions, and input/output schemas. The LLM uses this to discover what's possible and plan its actions.

### Key methods

| Method | Purpose |
|--------|---------|
| `commands.list` | Discover available commands |
| `commands.describe` | Get details about a specific command |
| `execute` | Run a single command with optional verification |
| `pipeline` | Run a multi-step pipeline |
| `project.load` | Load a spec file |
| `project.plan` | Generate an execution plan from the loaded spec |
| `project.approve` | Approve and execute the plan |
| `project.reject` | Reject the plan |
| `context.get` / `context.set` | Read/write scoped context |
| `checkpoint.save` / `checkpoint.restore` | Save/restore state snapshots |

### Run the full agent demo

Demo 04 exercises the complete agent autonomy loop — an LLM discovers commands, plans an approach, gathers data from GitHub, and writes a health report:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh \
  agsh run examples/demo/04-agent-autonomy/project.agsh.yaml \
  --param repo=golang/go
```

---

## 5. Watch the Agent with the Inspector UI

When an agent is running autonomously, you need visibility into what it's doing. The **Inspector** is a web-based UI embedded directly in the agsh binary — no separate install needed.

### Start agsh with the Inspector

```bash
docker-compose -f docker/docker-compose.yaml exec agsh \
  agsh --inspector --mode=agent
```

Or with a custom port:

```bash
agsh --inspector-port=8080
```

Or via environment variable:

```bash
AGSH_INSPECTOR=4200 agsh --mode=agent
```

Then open your browser at **http://localhost:4200**.

### What you see

The Inspector provides five views:

**Dashboard** — A live overview showing the current task, step progress, and quick stats. This is your starting point to understand what the agent is doing right now.

**Plan View** — The full execution plan with each step's description, risk level, and verification requirements. Before the agent starts work, you can review the plan here and approve or reject it.

**Event Stream** — A chronological, filterable log of every runtime event: commands starting and finishing, verification results, context changes, checkpoints. This is where you debug when something goes wrong.

**Context Explorer** — Browse all context scopes (project, session, step, history) as a tree. See what data the agent is working with, what it has accumulated from previous steps, and what metadata the spec provides.

**History & Checkpoints** — View past runs and saved state snapshots. Compare checkpoint states to understand how the agent's working data changed over time.

### Approve or reject from the UI

If the approval mode is set to `plan` or `always` in your config, the Inspector shows approve/reject buttons when the agent generates a plan. You can review the plan in the Plan View and intervene directly from the browser — useful when you're monitoring from a phone while the agent runs on a server.

### Inspector + Docker Compose

To expose the Inspector port from Docker, add a port mapping to your `docker-compose.yaml`:

```yaml
services:
  agsh:
    # ... existing config ...
    ports:
      - "4200:4200"
    environment:
      - AGSH_INSPECTOR=4200
```

Then run:

```bash
docker-compose -f docker/docker-compose.yaml up -d --build
```

The Inspector is now reachable at `http://localhost:4200` from your host machine.

---

## 6. Write Your Own Spec

Once you're comfortable running demos, create your own spec. agsh includes templates to get started:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh \
  agsh init --template=weekly-report
```

This creates a `project.agsh.yaml` in your workspace. Edit it to match your needs:

```yaml
apiVersion: agsh/v1
kind: ProjectSpec

meta:
  name: "my-first-spec"
  description: "A simple task for the agent"
  author: "me"
  tags: ["learning"]

goal: |
  List all files in the workspace and write a summary
  of what you find to output.md.

constraints:
  - "Only write to output.md"
  - "Complete within 30 seconds"

success_criteria:
  - type: "not_empty"
    target: "output"
    message: "Output must not be empty"

allowed_commands:
  - "fs:*"

output:
  path: "./output.md"
  format: "markdown"

params: []
```

Run it:

```bash
docker-compose -f docker/docker-compose.yaml exec agsh \
  agsh run project.agsh.yaml
```

### Spec anatomy

| Field | Purpose |
|-------|---------|
| `goal` | Natural language description of what the agent should achieve |
| `constraints` | Hard rules the agent must follow (read-only, time limits, etc.) |
| `guidelines` | Soft suggestions for quality and style |
| `success_criteria` | Automated checks run against the output after execution |
| `allowed_commands` | Glob patterns controlling which commands the agent can use |
| `output` | Where to write the final result |
| `params` | User-configurable parameters with defaults |

---

## 7. Security and Risks of Running agsh in Docker

agsh is designed to let AI agents execute commands autonomously. This is powerful but carries inherent risk. The Docker-based deployment is a key part of the security model.

### What Docker provides

**Process isolation.** The agsh container runs as a separate process with its own filesystem, network namespace, and user space. A misbehaving agent cannot directly affect your host system.

**Non-root execution.** The container runs as a dedicated `agsh` user, not root. Even inside the container, the agent has limited privileges.

**Filesystem boundaries.** The agent operates within `/workspace`. Only directories you explicitly mount via Docker volumes are accessible. Your host filesystem is not exposed unless you mount it.

**Network isolation.** The container uses a bridge network (`agsh-net`). The agent cannot reach arbitrary services on your host network unless you configure it to.

### What Docker does NOT provide

**Docker is not a security sandbox.** Container escapes are a known class of vulnerability. Docker isolates by convention, not by hardware boundary. Do not treat Docker as equivalent to a VM or hardware sandbox when running untrusted code.

**Volume mounts bypass isolation.** Any directory you mount into the container is fully accessible to the agent. Mounting your home directory, SSH keys, cloud credentials, or `/var/run/docker.sock` gives the agent access to those resources. Only mount what the agent needs.

**Environment variables are visible inside the container.** Secrets passed via `GITHUB_TOKEN` or other env vars are readable by any process in the container, including the agent. The agent needs credentials to call APIs, but understand that it has full access to any secret you inject.

**Network access is broad by default.** Unless you restrict outbound traffic with firewall rules or Docker network policies, the agent can make HTTP requests to any reachable endpoint. agsh has an HTTP domain allowlist (`http.allowed_domains` in config), but this only applies to agsh's own `http:get` and `http:post` commands — it does not restrict other network access at the OS level.

### Risks to understand

**Autonomous execution means less human oversight.** In agent mode, the LLM decides which commands to run and in what order. While the approval flow lets you review plans before execution, individual commands within an approved plan execute without further confirmation.

**LLM behavior is non-deterministic.** The same spec can produce different plans on different runs. An agent might take unexpected actions, especially with broad `allowed_commands` patterns like `"*"`.

**Data exfiltration.** If the agent has network access and your data is mounted in the workspace, it could in principle send data to external endpoints. Use the HTTP allowlist and restrict `allowed_commands` to limit this surface.

**Resource consumption.** An agent in a loop can consume CPU, memory, and disk. Docker resource limits (`--memory`, `--cpus`) can mitigate this but are not set in the default compose file.

### Hardening recommendations

1. **Restrict allowed commands.** Use specific patterns (`"fs:read"`, `"github:repo:info"`) instead of broad globs (`"fs:*"`, `"*"`). Only grant the commands the spec actually needs.

2. **Use the approval flow.** Set `approval.mode: "plan"` or `"always"` in your config so you review every plan before execution. Start with `"always"` until you trust your specs.

   ```yaml
   # .agsh/config.yaml
   approval:
     mode: "plan"
     timeout: 300
   ```

3. **Limit volume mounts.** Only mount the specific directory the agent needs. Never mount sensitive directories (home, `.ssh`, `.aws`, Docker socket).

   ```yaml
   volumes:
     - ./project-data:/workspace   # only this directory
   ```

4. **Set Docker resource limits.** Prevent runaway resource usage:

   ```yaml
   services:
     agsh:
       deploy:
         resources:
           limits:
             cpus: "2.0"
             memory: "1G"
   ```

5. **Use the HTTP allowlist.** Restrict which domains the agent can reach:

   ```yaml
   # .agsh/config.yaml
   http:
     allowed_domains:
       - api.github.com
   ```

6. **Restrict network access.** For sensitive workloads, use Docker's `internal` network driver to block all external traffic, then allowlist specific endpoints via a proxy.

7. **Rotate credentials.** Use short-lived tokens. If you pass a `GITHUB_TOKEN`, scope it to the minimum required permissions and set a short expiry.

8. **Monitor with the Inspector.** Run the Inspector UI and watch what the agent does, especially during early experiments. The Event Stream view shows every command the agent executes in real time.

9. **Run specs in read-only mode first.** Before granting write access, test your spec with read-only commands to verify the agent's behavior matches your expectations.

10. **Do not expose the Inspector to the public internet.** The Inspector has no authentication. It's designed for local use or trusted networks. If you need remote access, put it behind a VPN or authenticated reverse proxy.

### Summary of the trust model

| Layer | What it controls | Limitation |
|-------|-----------------|------------|
| **Spec** (`allowed_commands`) | Which agsh commands the agent can invoke | Agent can use any allowed command creatively |
| **Config** (`approval.mode`) | Whether humans must approve plans | Only gates at plan level, not individual commands |
| **Config** (`http.allowed_domains`) | Which domains `http:get`/`http:post` can reach | Only applies to agsh HTTP commands, not OS-level |
| **Docker** (container isolation) | Filesystem, process, basic network isolation | Not a hard security boundary; volume mounts bypass it |
| **Docker** (non-root user) | Limits in-container privilege | Still has full access to mounted volumes |

The security model works in layers. No single layer is sufficient on its own. Use all of them together, and start with restrictive settings that you relax as you gain confidence.

---

## Next Steps

- Read the [Architecture Guide](architecture.md) for the full system design
- Study the [Demo Specs](demo-specs.md) for progressively complex examples
- Read the [Inspector Guide](inspector-gui.md) for detailed UI documentation
- Browse `templates/` for ready-made spec templates you can customize
