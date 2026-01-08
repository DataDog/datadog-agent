# Process Manager Presentation Notes

> **Duration**: 15-20 minutes + Q&A  
> **Audience**: [Adjust based on your audience - technical team / stakeholders / mixed]

---

## Slide 1: Title Slide

**Title**: Datadog Agent Process Manager  
**Subtitle**: Unified Process Lifecycle Management  
**Speaker**: José Manuel Almaza

*No speaker notes needed - just introduce yourself and the topic*

---

## Slide 2: The Problem (1-2 min)

### Visual
Use: `diagrams/current-architecture.png`

### Speaker Notes

> "Let me start with the challenge we're solving.
>
> Today, the Datadog Agent runs as **5+ separate OS services**: core-agent, trace-agent, process-agent, security-agent, and system-probe. Each one is managed directly by the operating system.
>
> On Linux, that's systemd. On macOS, launchd. On Windows, the Service Control Manager.
>
> **The problem?** Each platform handles things differently:
> - Different restart behavior per distro
> - Different dependency models
> - Different ways to configure resource limits
> - Different failure recovery mechanisms
>
> When something goes wrong, you're debugging OS-level service configuration, not agent behavior. And if a customer wants to tweak restart policies, they're editing systemd unit files—not something we control."

### Key Points to Hit
- 5+ services per host (show the diagram)
- Platform fragmentation (Linux/macOS/Windows)
- Configuration lives in OS-level files we don't fully control
- Hard to maintain consistency across platforms

---

## Slide 3: The Solution (2-3 min)

### Visual
Use: `diagrams/target-architecture.png` (ideally side-by-side with current architecture)

### Speaker Notes

> "Here's what we're building: the **Datadog Agent Process Manager**.
>
> Instead of 5+ OS services, there's **one**. The OS sees a single `datadog-agent` service. Inside, our process manager—`dd-procmgrd`—supervises all the agent components as child processes.
>
> Think of it as **systemd inside our agent**—but consistent across all platforms.
>
> The OS just needs to do one thing: start and restart our process manager on failure. Everything else—restart policies, dependencies between components, health monitoring, resource limits—is **agent-controlled**.
>
> **What does this mean practically?**
> - Same behavior whether you're on RHEL, Ubuntu, macOS, or Windows
> - We control the restart policies, not OS defaults
> - Dependencies are explicit: trace-agent knows it needs core-agent
> - Health checks are built-in, not bolted on
>
> And for users? `systemctl start datadog-agent` still works. It's a transparent change."

### Key Points to Hit
- Single OS service = simplified management
- Agent-controlled policies (no more editing unit files)
- Cross-platform consistency (same YAML configs everywhere)
- Backward compatible (service name unchanged)

---

## Slide 4: Key Benefits (1-2 min)

### Visual
Table or bullet list:

| Benefit | What It Means |
|---------|---------------|
| **Consistency** | Same restart/health behavior on all platforms |
| **Control** | Policies defined in agent config, not OS files |
| **Observability** | Built-in metrics for all managed processes |
| **Reliability** | Intelligent restart with exponential backoff |
| **Simplicity** | One service to manage instead of five |

### Speaker Notes

> "Let me highlight the key benefits:
>
> **Consistency**: Whether you're running on Ubuntu, RHEL, macOS, or eventually Windows—restart policies, health checks, and dependencies work the same way.
>
> **Control**: Everything is configured through YAML files we own. No more 'it behaves differently because of systemd version X.'
>
> **Observability**: We can emit metrics for every managed process—CPU usage, memory, restart counts, health status. This is much harder when they're separate OS services.
>
> **Reliability**: We implement proper exponential backoff and circuit breakers. If a process crashes repeatedly, we don't restart it infinitely—we enter a 'crashed' state that requires manual intervention. This prevents runaway resource consumption.
>
> **Simplicity**: One service. One place to look. One thing to restart."

---

## Slide 5: Architecture Overview (2-3 min)

### Visual
Use: `diagrams/hexagonal-architecture-overview.png`

### Speaker Notes

> "Let me walk you through the architecture.
>
> We built this using **hexagonal architecture**—ports and adapters pattern. This was intentional because we need to support multiple platforms.
>
> **On the left, the inputs**:
> - **gRPC API**: Primary interface for the CLI and programmatic access
> - **REST API**: Optional HTTP/JSON interface (disabled by default)
> - The CLI tool `dd-procmgr` talks to the daemon over Unix socket
>
> **In the center, the core**:
> - **Application Layer**: Use cases like StartProcess, StopProcess, LoadConfig
> - **Domain Layer**: The business logic—supervision, health monitoring, dependency resolution
> - **Ports**: Interfaces that define *what* we need, not *how* it's implemented
>
> **On the right, the implementations**:
> - Linux implementation with cgroups v2 for resource limits
> - Windows implementation would use Job Objects (future)
> - Storage for process state
>
> This separation means adding macOS or Windows support is about implementing the right-side adapters—the core logic stays the same."

### Key Points to Hit
- Hexagonal architecture = platform portability
- gRPC as primary API (CLI uses it)
- Core domain is platform-agnostic
- Platform-specific code isolated to infrastructure layer

---

## Slide 6: Core Domain Services (2-3 min)

### Visual
Bullet list or simple diagram showing the services:

```
Domain Services:
├── Process Supervision Service (central coordinator)
├── Health Monitoring Service (HTTP/TCP/Exec probes)
├── Dependency Resolution Service (topological sort)
├── Conflict Resolution Service (stable vs experimental)
└── Socket Activation Service (on-demand startup)
```

### Speaker Notes

> "The domain layer has five main services:
>
> **Process Supervision Service** is the heart of the system. It:
> - Watches for process exits (event-driven, not polling)
> - Decides whether to restart based on policy
> - Calculates exponential backoff delays
> - Enforces start limits to prevent restart storms
>
> **Health Monitoring Service** runs configurable health probes—Kubernetes-style:
> - HTTP probes (hit `/health` endpoint)
> - TCP probes (can you connect to the port?)
> - Exec probes (run a command, check exit code)
>
> If a process becomes unhealthy, we can automatically restart it.
>
> **Dependency Resolution Service** handles startup order:
> - Uses Kahn's algorithm for topological sort
> - Supports `requires` (hard dependency), `wants` (soft), `binds_to` (tight coupling)
> - Detects circular dependencies at load time
>
> **Conflict Resolution** ensures only one variant runs at a time—critical for stable vs experimental agent switching during upgrades.
>
> **Socket Activation** lets us start services on-demand. For example, trace-agent could start only when the first trace arrives."

---

## Slide 7: Process Lifecycle (3-4 min)

### Visual
Use: `diagrams/process-states.png` and `diagrams/restart-decision-flow.png`

### Speaker Notes

> "Let me walk through the process lifecycle.
>
> A process goes through several states:
> - **Created**: Registered but not started
> - **Starting**: Being spawned
> - **Running**: Has a PID, executing
> - **Stopped**: Explicitly stopped by user—no automatic restart
> - **Exited**: Exited cleanly (code 0)
> - **Failed**: Exited with non-zero code
> - **Crashed**: Start limit exceeded—terminal state, needs manual intervention
>
> *[Show restart decision flow diagram]*
>
> When a process exits, here's the decision tree:
> 1. Was it explicitly stopped? → Stay stopped
> 2. Exit code 0 or non-zero? → Exited vs Failed state
> 3. What's the restart policy?
>    - `never`: Don't restart
>    - `always`: Always restart
>    - `on-failure`: Restart only if Failed
>    - `on-success`: Restart only if Exited (rare use case)
> 4. Have we exceeded the start limit? → Crashed state (circuit breaker)
> 5. Otherwise → Apply exponential backoff and restart
>
> **The exponential backoff** prevents restart storms: 1s → 2s → 4s → 8s → 16s, capped at 60s.
>
> **The start limit** is our circuit breaker: if a process restarts more than N times in M seconds, it enters Crashed state and requires manual intervention. This prevents a broken process from consuming resources indefinitely."

### Key Points to Hit
- Clear state machine (show the diagram)
- Restart policies (never, always, on-failure, on-success)
- Exponential backoff prevents restart storms
- Start limit = circuit breaker for broken processes

---

## Slide 8: Configuration Example (2 min)

### Visual
Show the YAML config (simplified version):

```yaml
# /etc/datadog-agent/process-manager/processes.d/datadog-agent.yaml

description: Datadog Agent
command: /opt/datadog-agent/bin/agent/agent
args: [run, -c, /etc/datadog-agent/datadog.yaml]

user: dd-agent
restart: on-failure
start_limit_burst: 5
start_limit_interval_sec: 10

conflicts:
  - datadog-agent-exp    # Only one variant runs

wants:                   # Soft dependencies
  - datadog-agent-trace
  - datadog-agent-process

health_check:
  type: http
  http_endpoint: http://localhost:5000/health
  interval: 30
  restart_after: 3       # Kill & restart after 3 failures
```

### Speaker Notes

> "Configuration is simple YAML. One file per process in `/etc/datadog-agent/process-manager/processes.d/`.
>
> Let me highlight a few things:
>
> - **user: dd-agent** — The daemon runs as root but drops privileges for child processes
> - **restart: on-failure** — Restart only on crashes, not clean exits
> - **start_limit_burst/interval** — Our circuit breaker: max 5 restarts in 10 seconds
> - **conflicts** — Mutual exclusion with experimental agent
> - **wants** — Soft dependencies, auto-start these but continue if they fail
> - **health_check** — HTTP probe every 30 seconds, restart after 3 consecutive failures
>
> This is much more readable than systemd unit files, and it works the same on every platform."

---

## Slide 9: Live Demo / CLI Walkthrough (3-4 min)

### Option A: Live Demo

```bash
# Show running processes
dd-procmgr list

# Get detailed status
dd-procmgr describe datadog-agent

# Stop a process (watch it get restarted by policy)
dd-procmgr stop trace-agent

# Watch the restart happen
dd-procmgr list

# Check resource usage
dd-procmgr stats datadog-agent
```

### Speaker Notes (for demo)

> "Let me show you the CLI in action.
>
> `dd-procmgr list` shows all managed processes with their state, PID, and uptime.
>
> `dd-procmgr describe` gives detailed info: restart count, health status, resource usage.
>
> Now watch what happens when I stop trace-agent... 
> *[stop it]*
> 
> The supervision service detects it's gone, checks the restart policy, and brings it back up.
>
> `dd-procmgr stats` shows real-time resource usage from cgroups."

### Option B: Walkthrough (if demo not possible)

Show CLI output screenshots or mock outputs:

```
$ dd-procmgr list
NAME                  STATE     PID     UPTIME      RESTARTS
datadog-agent         Running   1234    2h 15m      0
trace-agent           Running   1235    2h 15m      0
process-agent         Running   1236    2h 15m      0
security-agent        Running   1237    2h 15m      0
system-probe          Running   1238    2h 15m      0
```

---

## Slide 10: Migration Strategy (2 min)

### Visual
Timeline or phases:

```
Phase 1 (Current): Linux bare-metal/VM implementation ← We are here
Phase 2: Controlled rollout via Fleet Automation
Phase 3: Linux GA (default for new installs)
Phase 4: macOS support
Phase 5: Windows support
Phase 6: Kubernetes consideration
```

### Speaker Notes

> "We're taking a phased approach.
>
> **Phase 1**—where we are now—targets Linux bare-metal and VM environments. No containers, no Kubernetes. This is our PoC scope.
>
> **Phase 2** will be controlled rollout using Fleet Automation policies. FA will determine which hosts use the process manager. The installer reads the policy and configures the right systemd services.
>
> **Phase 3** makes it the default for new Linux installations, with gradual rollout to existing ones.
>
> **Phases 4 and 5** add macOS and Windows support.
>
> **Phase 6** is Kubernetes—we've done a detailed analysis. The short answer: it *can* work in K8s and has some benefits, but K8s already provides much of what we offer. We'll evaluate after bare-metal is solid.
>
> **Critical point**: This is **backward compatible**. The service is still called `datadog-agent`. `systemctl start datadog-agent` works. Rollback is possible—same agent version, different execution model."

---

## Slide 11: Implementation Details (optional, backup slide)

### Visual
Technology stack:

| Component | Technology |
|-----------|------------|
| Language | Rust |
| Async Runtime | tokio |
| gRPC | tonic |
| REST (optional) | axum |
| Resource Limits | cgroups v2 (Linux) |
| IPC | Unix domain socket |

### Speaker Notes

> "For those interested in implementation details:
>
> We chose **Rust** for several reasons:
> - Low overhead—this runs on every agent host
> - Memory safety without garbage collection pauses
> - Excellent async support via tokio
> - Rich ecosystem (tonic for gRPC, axum for REST)
>
> The daemon uses **tokio** for async event handling—no polling. Process exits come through channels, health check timers are async, everything is non-blocking.
>
> Resource limits use **cgroups v2** on Linux. If cgroups aren't available, we fall back to rlimit with a warning.
>
> CLI communicates with daemon via **Unix domain socket** by default. File permissions handle access control—simple and secure."

---

## Slide 12: Closing & Next Steps (1 min)

### Speaker Notes

> "To wrap up:
>
> **What we've built**: A unified process manager that gives us consistent behavior across platforms, intelligent restart handling, health monitoring, and proper dependency management.
>
> **Current status**: PoC is complete. RFC is ready for review.
>
> **What I need from you**:
> - Feedback on the design and approach
> - Alignment on the migration strategy
> - Any concerns we should address before moving forward
>
> The RFC and code are available in the branch for a deeper dive.
>
> Questions?"

---

# Q&A Backup Slides

## Q: "Why not just use systemd?"

### Answer

> "Great question. systemd is excellent, and we modeled many of our features after it. But there are three key reasons:
>
> **1. Cross-platform consistency**: systemd is Linux-only. macOS uses launchd, Windows uses SCM. Each has different semantics for restarts, dependencies, and health checks. We'd need to maintain three different configurations with subtly different behavior.
>
> **2. Agent-controlled policies**: With systemd, restart policies live in unit files owned by the package. Customers can edit them, distros can have different defaults, and Fleet Automation can't easily adjust them. With our process manager, policies are in our configuration—we control them.
>
> **3. Health checks**: systemd has limited health check support (mainly sd_notify). We needed Kubernetes-style probes—HTTP, TCP, Exec—with configurable thresholds and restart-after-unhealthy semantics.
>
> We still *use* systemd for the top-level service. It just manages one thing: our process manager. Everything else is internal."

---

## Q: "What happens if dd-procmgrd crashes?"

### Answer

> "The OS service manager (systemd) restarts it—that's the one thing we rely on the OS for.
>
> When dd-procmgrd starts up, it reloads configuration from `/etc/datadog-agent/process-manager/processes.d/`. Any process defined in config files is recovered.
>
> **Important caveat**: Processes created via CLI (not config files) are not persisted. If someone used `dd-procmgr create` to add a temporary process, that's lost on restart. But all standard agent processes come from config files, so they're always recovered."

---

## Q: "Why Rust instead of Go?"

### Answer

> "We considered both. Go would have been fine, and it matches our agent codebase.
>
> We chose Rust for a few reasons:
>
> **1. Resource efficiency**: This daemon runs on every host. Rust's zero-cost abstractions and no GC mean lower memory overhead and no GC pauses.
>
> **2. Async ecosystem**: tokio is excellent for our use case—many concurrent operations (watching processes, running health checks, handling API requests) without thread-per-task overhead.
>
> **3. Safety**: Process management involves a lot of unsafe operations (spawning processes, handling signals, cgroups). Rust's ownership model catches bugs at compile time.
>
> **4. Learning opportunity**: Building something new in a new language for the team.
>
> That said, the CLI could have been Go—it's just gRPC calls. We kept it in Rust for consistency."

---

## Q: "How do we rollback if something goes wrong?"

### Answer

> "Rollback doesn't require a version change—the same agent package supports both execution models.
>
> **How it works**:
> 1. Update the Fleet Automation policy to disable process manager for affected hosts
> 2. Installer runs (scheduled or manually triggered)
> 3. Installer stops `datadog-agent.service` (the process manager version)
> 4. Installer reconfigures systemd: disables process manager service, enables legacy multi-service unit files
> 5. Installer starts the legacy services
>
> The agent continues running with the same version, just managed by systemd directly instead of our process manager.
>
> This is automatic through FA policies or can be done manually on a single host."

---

## Q: "What about Kubernetes?"

### Answer

> "We've done a detailed analysis—there's a document in the repo.
>
> **Short answer**: The process manager *can* work in K8s, but it's not the primary target.
>
> **Why K8s is different**: Kubelet already provides restart policies, health probes (liveness/readiness), and resource limits at the container level. Running dd-procmgrd inside a container creates two layers of process management.
>
> **Where it might help**:
> - Faster restarts (process restart vs container restart)
> - Consistent behavior between bare-metal and K8s deployments
> - Tightly coupled processes that need to start/stop together
>
> **Our approach**: Focus on bare-metal/VM first (Phase 1-3). Evaluate K8s later based on what we learn. If customers want unified behavior across all environments, we have a path."

---

## Q: "What's the performance impact?"

### Answer

> "Actually, it should be a net improvement:
>
> **Before**: 5+ systemd services, each with its own service file, socket, and overhead.
>
> **After**: 1 systemd service, one daemon managing child processes directly.
>
> The process manager itself is lightweight—Rust with async IO, minimal memory footprint. Health check probes are non-blocking. Process watching uses `waitpid`, not polling.
>
> Resource limits are enforced via cgroups v2, which is the same mechanism systemd uses—no additional overhead there.
>
> We haven't done formal benchmarking yet, but architecturally there's less overhead, not more."

---

## Q: "How do you handle orphan processes?"

### Answer

> "When we stop a process, we don't just kill the main PID—we kill everything in its cgroup.
>
> This handles cases where a process spawns children. The cgroup contains the parent and all its descendants, so `cgroup.kill` gets them all.
>
> If cgroups aren't available (older kernels), we fall back to process group killing with SIGKILL to the process group.
>
> This is similar to systemd's `KillMode=control-group` behavior."

---

## Q: "What about socket activation? Is that really needed?"

### Answer

> "Socket activation is optional but valuable for specific cases.
>
> **Use case**: trace-agent receives APM traces on a socket. With socket activation:
> 1. Process manager creates the socket at startup
> 2. trace-agent doesn't start yet
> 3. First trace arrives → process manager starts trace-agent
> 4. trace-agent inherits the socket, handles the trace
>
> **Benefits**:
> - Zero-downtime restarts (socket stays open during restart)
> - On-demand startup (saves resources if traces are rare)
> - Connection queuing (connections wait while process restarts)
>
> We pass the socket via `LISTEN_FDS` (systemd-compatible) and `DD_APM_NET_RECEIVER_FD` for trace-agent compatibility."

---

## Q: "How does the stable/experimental agent switching work?"

### Answer

> "This is the `conflicts` directive in action.
>
> Both `datadog-agent.yaml` and `datadog-agent-exp.yaml` declare conflicts with each other:
>
> ```yaml
> # datadog-agent.yaml
> conflicts:
>   - datadog-agent-exp
>
> # datadog-agent-exp.yaml  
> conflicts:
>   - datadog-agent
> ```
>
> When you start one, the conflict resolver:
> 1. Checks if the conflicting process is running
> 2. Sends SIGTERM to stop it gracefully
> 3. Waits up to `timeout_stop_sec` (default 90s)
> 4. Sends SIGKILL if needed
> 5. Only then starts the new process
>
> This ensures exactly one variant runs at a time—critical for Fleet Automation upgrades where we need to test experimental before promoting to stable."

---

## Timing Guide

| Section | Duration | Cumulative |
|---------|----------|------------|
| Title + Opening Hook | 2 min | 2 min |
| The Solution | 3 min | 5 min |
| Key Benefits | 2 min | 7 min |
| Architecture | 3 min | 10 min |
| Process Lifecycle | 3 min | 13 min |
| Config Example | 2 min | 15 min |
| Demo/Walkthrough | 3 min | 18 min |
| Migration & Closing | 2 min | 20 min |

**If running short on time**: Cut or abbreviate the Demo section  
**If running long**: Skip the detailed architecture slide, reference it for Q&A

---

## Checklist Before Presenting

- [ ] Test the demo environment (if doing live demo)
- [ ] Have backup static slides in case demo fails
- [ ] Know where all diagram PNGs are located
- [ ] Review the Q&A answers
- [ ] Have the RFC open for reference
- [ ] Test screen sharing with the diagrams

Good luck! 🎯

