# q_branch Development Rules

## Execution Modes

The q_branch infrastructure supports two execution modes, auto-detected based on environment:

| Mode | Environment | How it works |
|------|-------------|--------------|
| VM | macOS, Linux+KVM | Commands run inside Lima VM via `limactl shell` |
| Direct | Linux (no KVM) | Commands run directly on host Docker |

Check current mode: `cd q_branch && ./dev.py status`

In **Direct mode** (Workspaces, containers), there is no Lima VM - Kind clusters run directly on the host. All `./dev.py` commands work identically in both modes.

## Fine-Grained Monitor Development

Use `./dev.py` for all fine-grained-monitor (fgm-*) development workflows:

```bash
cd q_branch/fine-grained-monitor

# Local development
./dev.py local build              # Build all release binaries
./dev.py local test               # Run tests
./dev.py local clippy             # Run clippy lints
./dev.py local viewer start       # Start fgm-viewer with default data
./dev.py local viewer start --data /path/to/file.parquet
./dev.py local viewer stop        # Stop fgm-viewer
./dev.py local viewer status      # Check fgm-viewer status

# Cluster deployment (Kind via Lima) - per-worktree isolated
./dev.py cluster deploy           # Build image, load to Kind, restart pods (creates cluster if needed)
./dev.py cluster status           # Show cluster pod status
./dev.py cluster viewer start     # Port-forward to viewer on first pod
./dev.py cluster viewer start --pod NAME  # Port-forward to specific pod
./dev.py cluster viewer stop      # Stop viewer port-forward
./dev.py cluster list             # List all fgm-* clusters
./dev.py cluster create           # Create Kind cluster for this worktree
./dev.py cluster destroy          # Destroy Kind cluster for this worktree
./dev.py cluster mcp setup        # Setup MCP server for this worktree's cluster
./dev.py cluster mcp start        # Start MCP port-forward
./dev.py cluster mcp stop         # Stop MCP port-forward

# Benchmarking
./dev.py bench --filter <name>    # Run specific benchmark in background
./dev.py bench --full-suite       # Run all benchmarks in background
./dev.py bench wait <guid>        # Wait for benchmark and show results
./dev.py bench list               # List recent benchmark runs
```

**Prefer dev.py over raw commands** - it handles mode detection (VM vs Direct), image loading, Kind cluster operations, port management, and per-worktree isolation automatically.

### Per-Worktree Isolation

Each git worktree gets its own isolated Kind cluster:
- Cluster name: `fgm-{worktree-basename}` (e.g., `fgm-beta-datadog-agent`)
- API port: Deterministic based on worktree name (6443-6447)
- Data directory: `/var/lib/fine-grained-monitor/{worktree-basename}/`
- Image tag: `fine-grained-monitor:{worktree-basename}`

Multiple worktrees can run concurrently without conflicts.

### Benchmarking

**Generate benchmark data first**, then run benchmarks:

```bash
# Generate data with two scenarios: realistic or stress
cargo run --release --bin generate-bench-data -- --scenario realistic --duration 1h
cargo run --release --bin generate-bench-data -- --scenario stress --duration 1h

# Run benchmarks with generated data
BENCH_DATA=testdata/bench/realistic cargo bench
BENCH_DATA=testdata/bench/stress cargo bench

# Run specific benchmark
BENCH_DATA=testdata/bench/realistic cargo bench -- scan_metadata
```

**Available benchmarks:**
- `scan_metadata` - Startup path, measures parquet file scanning
- `get_timeseries_single_container` - Single container timeseries query
- `get_timeseries_all_containers` - All containers timeseries query

**Available data scenarios:**
- `realistic` - Stable workload: ~20 containers, 2-3 pod restarts/day, ~150-200 MB/day
- `stress` - Heavy churn: ~50 containers, 5-7 restarts/day, container turnover, ~500-800 MB/day

**Duration examples:** `1h`, `6h`, `24h`, `2d`, `7d`

## Architecture

Local development uses the native architecture of the host machine:
- macOS (Apple Silicon): ARM64 via Lima VM
- Linux: Native architecture (amd64 or arm64)

**Do NOT specify `--platform`** in docker build commands during local testing loops unless cross-compiling.

## Kubernetes Cluster (Per-Worktree)

Each worktree has its own Kind cluster:
- **VM mode**: Cluster runs inside Lima VM (`gadget-k8s-host`) with API port-forwarded to host
- **Direct mode**: Cluster runs directly on host Docker

### MCP Server Setup

Run `./dev.py cluster mcp setup` to configure the kubernetes-mcp-server for this worktree's cluster. This creates:
- A dedicated kubeconfig at `~/.kube/mcp-fgm-{worktree}.kubeconfig`
- A project-scoped `.mcp.json` that points to this worktree's cluster

**Restart Claude Code after running setup-mcp** to pick up the new configuration.

### Prefer MCP Tools Over kubectl

**Use kubernetes-mcp-server tools** for all cluster interactions:
- `pods_list`, `pods_list_in_namespace` - List pods
- `pods_log` - Get pod logs
- `pods_get` - Get pod details
- `pods_delete` - Delete pods
- `pods_exec` - Execute commands in pods
- `pods_run` - Run new pods
- `resources_list`, `resources_get`, `resources_create_or_update`, `resources_delete` - Generic resource operations
- `helm_list`, `helm_install`, `helm_uninstall` - Helm operations
- `events_list` - List cluster events

**Only use kubectl via Bash when:**
- MCP tools don't support the operation (e.g., `kubectl apply -f`)
- You need complex label selectors or field selectors
- Debugging MCP connectivity issues

When using kubectl, use the worktree's context: `--context kind-fgm-{worktree-basename}`
(e.g., `--context kind-fgm-beta-datadog-agent`). Run `./dev.py cluster status` to see the current context.

### VM Operations (VM Mode Only)

In VM mode, the Kind cluster runs inside a Lima VM. For debugging or inspecting the VM directly:

```bash
limactl shell gadget-k8s-host -- <command>
limactl shell gadget-k8s-host -- docker images
limactl shell gadget-k8s-host -- kind get clusters
```

In Direct mode, these commands run directly on the host (no limactl needed).

### Common Workflows

**Check pod status:**
```
Use: pods_list_in_namespace(namespace="fine-grained-monitor")
```

**View pod logs:**
```
Use: pods_log(name="<pod-name>", namespace="<namespace>")
```

**Restart pods (e.g., after image update):**
```
Use: pods_delete(name="<pod-name>", namespace="<namespace>")
# DaemonSet/Deployment will recreate it
```

**Apply manifests** (MCP doesn't support file-based apply):
```bash
# Use the worktree's context (run ./dev.py cluster status to see it)
kubectl apply -f <file>.yaml --context kind-fgm-<worktree>
```
