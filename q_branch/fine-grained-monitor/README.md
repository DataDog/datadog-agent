# Fine-Grained Monitor

## What It Does

Fine-Grained Monitor (FGM) captures detailed container resource metrics at 1-second resolution in Kubernetes clusters. It runs as a DaemonSet (one pod per node), collecting metrics from every container and storing them in Parquet files for analysis.

Built in Rust, it's designed for debugging resource issues (CPU throttling, memory pressure, OOM kills) in the Datadog Agent—or any containerized workload. The monitor runs independently, so you can observe the Agent without affecting its behavior.

## Prerequisites

- Kubernetes cluster (local or cloud)
- `kubectl` configured
- Docker (for building container images)

## Architecture

FGM consists of four binaries, all built from the same Rust codebase:

| Binary | Deployment | Purpose |
|--------|------------|---------|
| `fgm` | DaemonSet container | Collects metrics, writes Parquet files |
| `fgm-viewer` | DaemonSet sidecar | Serves web UI and HTTP API |
| `fgm-consolidator` | DaemonSet sidecar (optional) | Merges small Parquet files |
| `fgm-mcp-server` | Deployment (1 replica) | Routes LLM queries to correct node |

All components run in the `fine-grained-monitor` namespace for easy cleanup and management.

**For detailed architecture, data flow, and design decisions, see [specs/design.md](specs/design.md)**

## Quick Start

```bash
cd q_branch/fine-grained-monitor

# Deploy cluster with FGM daemonset
./dev.py cluster deploy

# Open the metrics viewer
./dev.py cluster viewer start
```

## Running Scenarios

Scenarios are reproducible workloads that exhibit specific behaviors for investigation:

```bash
# List available scenarios
./scenario.py list

# Run a scenario
./scenario.py run memory-leak

# Check status
./scenario.py status

# View logs
./scenario.py logs

# Stop and clean up
./scenario.py stop <run_id>
```

**Available scenarios:**

| Scenario | Description |
|----------|-------------|
| `memory-leak` | Gradual memory growth to trigger pressure/OOM |
| `oom-kill` | Rapid allocation to trigger OOM killer |
| `crash-loop` | Container crash and restart cycles |
| `sigpipe-crash` | SIGPIPE-induced crash from broken pipe |
| `todo-app` | Multi-service app (frontend, backend, postgres) |

## Exporting Results

Export captured metrics for offline analysis or sharing:

```bash
# Export scenario data (creates .parquet and .html files)
./scenario.py export <run_id>

# Specify output path
./scenario.py export <run_id> -o my-analysis
# Creates: my-analysis.parquet, my-analysis.html
```

The exported HTML is self-contained—works offline with the full viewer UI.

## What It Captures

Per-container, every second:

| Category | Metrics |
|----------|---------|
| **CPU** | Usage (millicores), user/system split, throttling stats |
| **Memory** | Current usage, PSS, RSS, cgroup limits, pressure (PSI) |
| **I/O** | Read/write bytes, pressure stats |
| **Metadata** | Pod name, namespace, QoS class, container ID |

Output: Parquet files partitioned by date and pod, queryable with DuckDB/pandas/Spark.

## Viewer Features

The metrics viewer (`./dev.py cluster viewer start`) provides:

- Metric selection with fuzzy search
- Container filtering by namespace/QoS class
- Drag-to-zoom, scroll-wheel zoom, overview navigator
- Periodicity detection for recurring patterns (throttling cycles, GC, cron jobs)

## Project Structure

```
src/
├── main.rs              # Monitor binary (runs on nodes)
├── bin/fgm-viewer.rs    # Viewer binary (runs locally)
├── metrics_viewer/      # Web UI and analysis
└── observer/            # Metrics collection logic

scenarios/               # Reproducible test scenarios
deploy/                  # K8s deployment manifests
```

## Development

```bash
# Build locally
cargo build --release

# Run benchmarks
cargo bench 2>/dev/null

# Generate test data
cargo run --release --bin generate-bench-data -- --scenario realistic --duration 1h
```
