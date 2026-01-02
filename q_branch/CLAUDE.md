# q_branch Development Rules

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
./dev.py cluster forward          # Port-forward to cluster pod
./dev.py cluster forward-stop     # Stop port-forward
./dev.py cluster list             # List all fgm-* clusters
./dev.py cluster create           # Create Kind cluster for this worktree
./dev.py cluster destroy          # Destroy Kind cluster for this worktree
./dev.py cluster setup-mcp        # Setup MCP server for this worktree's cluster

# Benchmarking
./dev.py bench --filter <name>    # Run specific benchmark in background
./dev.py bench --full-suite       # Run all benchmarks in background
./dev.py bench wait <guid>        # Wait for benchmark and show results
./dev.py bench list               # List recent benchmark runs
```

**Prefer dev.py over raw commands** - it handles image loading into Lima VM, Kind cluster operations, port management, and per-worktree isolation automatically.

### Per-Worktree Isolation

Each git worktree gets its own isolated Kind cluster:
- Cluster name: `fgm-{worktree-basename}` (e.g., `fgm-beta-datadog-agent`)
- API port: Deterministic based on worktree name (6443-6447)
- Data directory: `/var/lib/fine-grained-monitor/{worktree-basename}/`
- Image tag: `fine-grained-monitor:{worktree-basename}`

Multiple worktrees can run concurrently without conflicts.

### Benchmarking

Benchmarks run in background to avoid blocking and allow multiple analysis passes on the same run:

```bash
# Run a specific benchmark (preferred for quick iterations)
./dev.py bench --filter get_timeseries_single_container
# Returns: Running benchmark a1b2c3d4
#          Wait: ./dev.py bench wait a1b2c3d4

# Run full suite (for comprehensive testing)
./dev.py bench --full-suite

# Wait for completion and see results
./dev.py bench wait a1b2c3d4

# List recent runs
./dev.py bench list
```

**Available benchmarks:**
- `scan_metadata` - Startup path, measures parquet file scanning
- `get_container_stats_cold` - Cold query including data loading
- `get_container_stats_warm` - Warm query from cache
- `get_timeseries_single_container` - Single container timeseries query
- `get_timeseries_all_containers` - All containers timeseries query
- `load_all_metrics` - Full dashboard load simulation

Benchmark data is auto-generated on first run. Logs are stored in `.dev/bench/<guid>/` and auto-cleaned after 30 days.

## Architecture: aarch64 (ARM64)

All local development and testing runs on Apple Silicon (aarch64/ARM64).

**Do NOT specify `--platform linux/amd64`** in docker build commands during local testing loops. The Lima VM, Kind cluster, and all containers run natively on ARM64.

## Kubernetes Cluster (Per-Worktree)

Each worktree has its own Kind cluster inside the Lima VM (`gadget-k8s-host`) with the API port-forwarded to the host.

### MCP Server Setup

Run `./dev.py cluster setup-mcp` to configure the kubernetes-mcp-server for this worktree's cluster. This creates:
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

### VM Operations

The Kind cluster runs inside a Lima VM. For debugging or inspecting the VM directly:

```bash
limactl shell gadget-k8s-host -- <command>
limactl shell gadget-k8s-host -- docker images
limactl shell gadget-k8s-host -- kind get clusters
```

### Common Workflows

**Check pod status:**
```
Use: pods_list_in_namespace(namespace="default")
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
