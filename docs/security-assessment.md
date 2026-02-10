# agsh Security Assessment

> Analysis of security boundaries, enforced controls, and known gaps in agsh
> and agsh-in-Docker.

**Date:** 2026-02-10
**Scope:** All code in the agsh repository as of Phase 1 completion
**Status:** Pre-production prototype — many declared controls are not yet
implemented in code

---

## 1. Threat Model

agsh is a CLI runtime designed to be driven by an LLM agent. This creates a
specific threat profile:

- **The primary operator is an AI agent**, not a human. The agent decides which
  commands to run, what paths to access, and what data to write. A misbehaving
  or compromised agent has the same access as a malicious user at a shell.
- **The attack surface is the command interface.** Every platform command
  (`fs:list`, `fs:read`, `fs:write`) is a potential entry point. Future
  commands (`github:*`, `http:*`) will expand this surface.
- **The trust boundary is between the spec (human intent) and the agent
  (autonomous execution).** The spec declares constraints; the runtime must
  enforce them.

### Actors

| Actor             | Trust Level | Access                                |
|-------------------|-------------|---------------------------------------|
| Human operator    | High        | Writes specs, configures sandbox      |
| LLM agent         | Low         | Executes commands via JSON-RPC/REPL   |
| Platform commands  | Medium     | Implement the PlatformCommand interface|
| External services | None        | GitHub API, HTTP endpoints (future)   |

### Key Assumption

The entire security model depends on agsh enforcing the boundaries declared in
project specs and config files. If the runtime does not enforce them, the agent
operates with no restrictions.

---

## 2. Critical Finding: Sandbox Is Declared But Not Enforced

This is the most important finding in this assessment.

The configuration file `.agsh/config.yaml` declares a sandbox:

```yaml
sandbox:
  workdir: /workspace
  allowed_paths:
    - /workspace
    - /tmp
  denied_paths:
    - /etc
    - /usr
  max_file_size: 10MB
```

**None of these restrictions are enforced in code.** The `internal/config/`
package does not exist yet (Phase 2). No filesystem command reads or checks
this configuration. The sandbox is purely aspirational.

### What this means in practice

An agent (or interactive user) can:

```
agsh> fs:read /etc/passwd
agsh> fs:read /root/.ssh/id_rsa
agsh> fs:write {"path": "/etc/cron.d/task", "content": "..."}
agsh> fs:list /
```

Every path on the filesystem accessible to the process owner is readable and
writable through agsh commands. The `allowed_paths` and `denied_paths`
restrictions are ignored.

Similarly, `max_file_size: 10MB` is never checked. A command can read a
multi-gigabyte file into memory or write unbounded data to disk.

### Risk

**Critical.** Users who configure the sandbox and assume it works are operating
under a false sense of security. This is worse than having no sandbox config at
all, because it creates misplaced confidence.

### Recommendation

Until sandbox enforcement is implemented, the config file should carry a
prominent warning that restrictions are not yet active. Alternatively, remove
the sandbox section from the default config to avoid implying enforcement.

---

## 3. Filesystem Command Vulnerabilities

All three filesystem commands share the same issues.

### 3.1 Path Traversal

**Files:** `pkg/platform/fs/read.go`, `pkg/platform/fs/list.go`,
`pkg/platform/fs/write.go`

The commands call `filepath.Abs()` to resolve the path, then pass it directly
to `os.ReadFile()`, `os.ReadDir()`, or `os.WriteFile()`. There is no
validation that the resolved path falls within an allowed directory.

```go
// fs/read.go — simplified
filePath, _ := extractFilePath(input)
filePath, _ = filepath.Abs(filePath)
data, _ := os.ReadFile(filePath)  // any path the process can access
```

Relative paths like `../../../etc/shadow` resolve to absolute paths and are
read without restriction.

**Severity:** Critical

### 3.2 Symlink Following

All filesystem operations follow symlinks by default. An attacker who can
create a symlink inside an allowed directory can use it to escape:

```bash
# Inside /workspace (would be "allowed"):
ln -s /etc/passwd /workspace/harmless.txt

# Then via agsh:
agsh> fs:read /workspace/harmless.txt
# Returns contents of /etc/passwd
```

The code uses `os.ReadFile()`, `os.ReadDir()`, and `os.WriteFile()`, all of
which follow symlinks. There is no call to `os.Lstat()` or
`filepath.EvalSymlinks()` to detect this.

**Severity:** Critical (enables sandbox escape even after path validation is
added, if symlinks are not handled)

### 3.3 No File Size Limits

`fs:read` calls `os.ReadFile()` which loads the entire file into memory.
`fs:write` calls `os.WriteFile()` with no size check on the content.

A malicious agent can:

- Read a huge file to exhaust memory (OOM kill)
- Write unbounded data to fill the disk

The declared `max_file_size: 10MB` is not enforced.

**Severity:** High (denial of service)

### 3.4 No Write Restrictions from Spec

Project specs declare `allowed_commands` (e.g., `fs:*`) but the heading-counter
spec also states the constraint `"Read-only access to workspace files (only
write output.md)"`. This constraint is a natural-language string — it is not
parsed or enforced by the runtime. The agent could write to any path.

**Severity:** High (relies entirely on agent compliance)

### Remediation for all filesystem issues

```
1. Load and parse sandbox config at startup
2. Before every fs operation:
   a. Resolve path to absolute with filepath.Abs()
   b. Resolve symlinks with filepath.EvalSymlinks()
   c. Check resolved path against allowed_paths (prefix match)
   d. Check resolved path against denied_paths (prefix match)
   e. For reads: stat the file and reject if size > max_file_size
   f. For writes: check content length against max_file_size
3. Return a generic error ("access denied") without leaking the full path
```

---

## 4. Context Store

**File:** `pkg/context/store.go`

### 4.1 No Scope-Level Access Control

The `ContextStore` interface exposes `Get`, `Set`, `Delete`, and `List` for any
scope. There is no distinction between scopes that should be read-only (like
`history`) and scopes that commands may freely modify.

A command can:

- Overwrite project goals: `store.Set("project", "goal", "new goal")`
- Erase history: `store.Delete("history", key)`
- Read credentials if stored: `store.Get("session", "GITHUB_TOKEN")`

**Severity:** High

### 4.2 No Input Validation on Scope or Key Names

The REPL's `context set` command passes user input directly to the store
without validating scope names or key names. If scope names are ever used in
file paths or bucket names, this could enable injection.

The BoltDB implementation uses scope as a bucket name, which is safe within
BoltDB itself, but this is an implicit rather than explicit guarantee.

**Severity:** Medium

### 4.3 Deserialization of Arbitrary Types

`store.Get()` unmarshals JSON into `any`:

```go
var result any
json.Unmarshal(data, &result)
```

Go's `encoding/json` deserializes into safe primitive types (`string`,
`float64`, `bool`, `map[string]any`, `[]any`), so this is not directly
exploitable. However, if future code performs type assertions or reflection on
stored values without validation, it could become a vector.

**Severity:** Low (currently safe, but fragile)

### Remediation

- Implement read-only enforcement for `project` and `history` scopes
- Whitelist scope names: `project`, `session`, `step`, `history`
- Validate key names against a safe pattern (`^[a-zA-Z0-9_.:-]+$`)
- Add caller identity to store operations for audit logging

---

## 5. Pipeline Execution

**File:** `pkg/context/pipeline.go`

### 5.1 No Execution Timeouts

`Pipeline.Run()` accepts a `context.Context` but the caller (`repl.go`,
`demo.go`) always passes `context.Background()` with no deadline. A command
that hangs (network call, reading from a pipe, etc.) blocks forever.

The config declares `approval.timeout: 300` but this is not wired into pipeline
execution.

**Severity:** High (denial of service)

### 5.2 No Command-Level Authorization

The pipeline executor resolves commands by name from the registry and runs
them. There is no check against the spec's `allowed_commands` list. A pipeline
step referencing a command outside the allowlist will execute anyway.

**Severity:** High (spec constraints not enforced)

### 5.3 Error Propagation Leaks System Details

Errors from commands bubble up with full system paths and OS error messages:

```
fs:read: open /etc/shadow: permission denied
```

This confirms file existence and reveals permission structure to the agent.

**Severity:** Medium (information disclosure)

### Remediation

- Wrap pipeline execution in `context.WithTimeout()`
- Check each step's command against `allowed_commands` before executing
- Sanitize errors before returning to the agent/user

---

## 6. Event Bus

**File:** `pkg/events/bus.go`

### 6.1 Unbounded History

The `MemoryBus` appends every event to a `history` slice with no size limit.
The config declares `history.max_entries: 10000` but this is not enforced. Over
long-running sessions, memory grows without bound.

**Severity:** Medium (memory exhaustion over time)

### 6.2 Potential Race in Unsubscribe

`Unsubscribe()` closes the subscriber channel while `Publish()` may be
concurrently sending to it. If a publish occurs between removing the subscriber
from the list and closing the channel, or if publish is iterating subscribers
while unsubscribe modifies the slice, this can panic with "send on closed
channel."

The current code holds `mu.Lock()` in both paths which prevents the race on
the slice, but `Publish()` sends to channels *while holding the lock*, and
`Unsubscribe()` closes channels *while holding the lock*. Since these use the
same mutex, they are serialized. However, the non-blocking send in `Publish`:

```go
select {
case sub.ch <- event:
default: // drop if full
}
```

combined with closing in `Unsubscribe` is safe *only because* both hold the
same mutex. If this is ever refactored to use separate locks for performance,
the race will surface.

**Severity:** Low (currently safe due to single mutex, fragile under
refactoring)

### 6.3 Event Data Can Contain Sensitive Information

Events carry an untyped `Data any` field. Commands can publish events
containing credentials, file contents, or API responses. If events are later
streamed to the inspector GUI over WebSocket, this data is exposed to anyone
with access to the inspector port.

**Severity:** Medium (information disclosure via inspector, once implemented)

### Remediation

- Implement circular buffer with configurable max size
- Document thread-safety invariants for future maintainers
- Define an event data schema; redact or omit sensitive fields

---

## 7. Docker Security

### 7.1 Container Configuration

**File:** `docker/Dockerfile`

Positive findings:

- Multi-stage build (build tools not in runtime image)
- Static binary with `CGO_ENABLED=0`
- Non-root user (`agsh`) created and used
- Dedicated `/workspace` and `/data` directories

Issues:

| Issue | Detail | Severity |
|-------|--------|----------|
| Unnecessary packages | `git` and `bash` installed in runtime image. These expand the attack surface and provide tools an attacker could use post-compromise. | Low |
| No read-only filesystem | Container filesystem is writable. An attacker could modify the `agsh` binary or install additional tools. | Medium |
| No resource limits | No `mem_limit`, `cpus`, `pids_limit` in compose. A runaway process can consume all host resources. | Medium |
| No seccomp/AppArmor profile | Default Docker seccomp profile applies, but no custom profile restricts syscalls to what agsh actually needs. | Low |

### 7.2 Docker Compose Configuration

**File:** `docker/docker-compose.yaml`

| Issue | Detail | Severity |
|-------|--------|----------|
| Credential via environment variable | `GITHUB_TOKEN=${GITHUB_TOKEN}` is visible in `docker inspect`, process listings, and container metadata. Use Docker secrets or mounted files instead. | High |
| No resource constraints | No `deploy.resources.limits` section. Container can use unlimited CPU, memory, and PIDs. | Medium |
| No read-only root filesystem | Missing `read_only: true`. Container can write to its own filesystem. | Medium |
| No `security_opt` | No `no-new-privileges:true`, no custom seccomp profile. | Low |
| Volume mount scope | `./workspace:/workspace` mounts the host's workspace directory. If the host path is misconfigured, it could expose sensitive host files. | Medium |

### 7.3 What Docker Does Protect

Even with the issues above, running in Docker provides meaningful isolation:

- **Process isolation.** The agent cannot see or signal host processes.
- **Network isolation.** The `agsh-net` bridge network limits connectivity.
  The container cannot reach the host network by default.
- **Filesystem boundary.** Path traversal within the container cannot reach
  the host filesystem (only container paths are accessible). This is the most
  important control — it limits the blast radius of the unimplemented sandbox.
- **User namespace.** The `agsh` user inside the container has no host
  privileges.

### 7.4 Docker as the De-Facto Sandbox

Because the application-level sandbox is not implemented, **Docker is currently
the only real security boundary.** This makes the container configuration
security-critical rather than defense-in-depth.

### Recommended Docker hardening

```yaml
services:
  agsh:
    read_only: true
    tmpfs:
      - /tmp:size=100M
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: "1.0"
          pids: 100
    secrets:
      - github_token
    environment:
      - AGSH_MODE=agent

secrets:
  github_token:
    file: ./secrets/github_token.txt
```

---

## 8. agsh Native (No Docker) vs. agsh in Docker

| Security Control | Native (no Docker) | Docker |
|-----------------|-------------------|--------|
| Filesystem isolation | **None.** Agent can access any file the user can. Path traversal gives full user-level access. | **Container boundary.** Path traversal is limited to the container's filesystem. Host files are only exposed via explicit volume mounts. |
| Network isolation | **None.** Future HTTP commands can reach any endpoint. | **Bridge network.** Container can only reach the network defined in compose. No host network access by default. |
| Process isolation | **None.** Agent runs as the user's process. | **Namespace isolation.** Container processes are isolated from host. |
| Resource limits | **None.** OOM or disk-fill affects the host directly. | **Configurable** (if set). Memory, CPU, PID limits protect the host. Not configured in current compose file. |
| Credential exposure | **High risk.** Agent can read `~/.ssh`, `~/.aws`, browser profiles, etc. | **Limited.** Only explicitly passed env vars and mounted volumes are visible. |
| Blast radius of compromise | **Full user account.** | **Container only.** Requires container escape for host access. |
| Sandbox config enforcement | Not implemented | Not implemented |
| Application-level path checks | Not implemented | Not implemented |

### Summary

Running agsh natively without Docker provides **no security boundary** between
the agent and the user's system. The agent has the same access as the user. Any
path traversal, unintended command, or agent misbehavior directly impacts the
host.

Running in Docker provides a **meaningful but imperfect boundary.** The
container filesystem limits the blast radius. However, volume mounts, passed
credentials, and the lack of resource limits create gaps that should be
hardened.

**Recommendation:** Always run agents in Docker. Do not run untrusted agents
natively until application-level sandbox enforcement is implemented.

---

## 9. JSON-RPC / Agent Mode (Future)

Agent mode is not yet implemented, but the architecture describes JSON-RPC over
stdin/stdout. Security considerations for the upcoming implementation:

- **Input validation.** Every JSON-RPC message must be validated against a
  schema before dispatch. Malformed or oversized messages should be rejected.
- **Method allowlisting.** Only defined methods should be callable. Unknown
  methods must return an error, not be silently ignored.
- **Argument validation.** Each method's parameters must be validated against
  the declared schema before execution.
- **Rate limiting.** The protocol should enforce limits on request frequency
  to prevent DoS.
- **Message size limits.** Both request and response sizes should be bounded to
  prevent memory exhaustion.
- **No shell interpretation.** Command arguments must never be passed through a
  shell. Use direct function calls only.

---

## 10. Summary of Findings by Severity

### Critical

| # | Finding | Component |
|---|---------|-----------|
| 1 | Sandbox config not enforced in code | fs commands, config |
| 2 | Path traversal in all filesystem commands | pkg/platform/fs/ |
| 3 | Symlink following bypasses path restrictions | pkg/platform/fs/ |

### High

| # | Finding | Component |
|---|---------|-----------|
| 4 | No file size limits (OOM / disk fill) | pkg/platform/fs/ |
| 5 | No execution timeouts in pipelines | pkg/context/pipeline.go |
| 6 | No scope-level access control in context store | pkg/context/store.go |
| 7 | `allowed_commands` from spec not enforced | pkg/context/pipeline.go |
| 8 | Credentials exposed via Docker env vars | docker/docker-compose.yaml |
| 9 | Running natively gives agent full user access | cmd/agsh/ |

### Medium

| # | Finding | Component |
|---|---------|-----------|
| 10 | Error messages leak system paths and state | fs commands, pipeline |
| 11 | Unbounded event history (memory growth) | pkg/events/bus.go |
| 12 | No Docker resource limits configured | docker/docker-compose.yaml |
| 13 | Container filesystem is writable | docker/Dockerfile |
| 14 | Event data can contain sensitive information | pkg/events/ |
| 15 | No audit logging of sensitive operations | All |

### Low

| # | Finding | Component |
|---|---------|-----------|
| 16 | Unnecessary packages in Docker runtime image | docker/Dockerfile |
| 17 | Context store deserialization uses `any` type | pkg/context/store.go |
| 18 | Event bus race condition fragile under refactor | pkg/events/bus.go |
| 19 | No structured logging implementation | All |

---

## 11. Prioritized Recommendations

### Before any agent runs outside a controlled test environment

1. **Implement path validation in filesystem commands.** Resolve the full path
   (including symlinks), then verify it falls within allowed directories.
2. **Enforce file size limits.** Check `stat.Size()` before read; check content
   length before write.
3. **Add execution timeouts.** Wrap pipeline execution in
   `context.WithTimeout()`.
4. **Harden Docker compose.** Add resource limits, read-only filesystem, and
   `no-new-privileges`. Move credentials to Docker secrets.

### Before any multi-user or networked deployment

5. **Enforce `allowed_commands` from project specs.** Check each pipeline step
   against the spec's command allowlist before execution.
6. **Implement scope-level access control** in the context store.
7. **Sanitize error messages.** Return generic errors to the agent; log details
   internally.
8. **Implement audit logging** for all command executions and context store
   writes.
9. **Cap event history** at the configured `max_entries` limit.

### Before production use

10. **Implement the full sandbox** as described in the architecture: path
    restrictions, network restrictions, resource limits, all enforced at the
    application level.
11. **Add rate limiting** to the command execution path.
12. **Define and enforce JSON-RPC message schemas** for agent mode.
13. **Security-review the inspector** before exposing it on any network
    interface (authentication, CORS, WebSocket origin checks).
