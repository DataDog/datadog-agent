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

## Scenarios

Scenarios are reproducible workloads that exhibit specific behaviors for investigation. The workflow is: **List → Run → View → Export**.

### The Core Loop

```bash
# 1. List available scenarios
./scenario.py list

# 2. Run a scenario (auto-stops after duration, default 30 min)
./scenario.py run todo-app-redis --duration 10

# 3. View metrics in browser
./dev.py cluster viewer start

# 4. Export for offline analysis or sharing
./scenario.py export
```

### Scenario Commands

| Command | Description |
|---------|-------------|
| `./scenario.py list` | List all available scenarios |
| `./scenario.py run <name>` | Deploy scenario to cluster |
| `./scenario.py run <name> --duration 60` | Run for 60 minutes then auto-stop |
| `./scenario.py status [run_id]` | Check scenario pod status |
| `./scenario.py logs [run_id]` | View scenario pod logs |
| `./scenario.py stop <run_id>` | Stop and clean up early |
| `./scenario.py export [run_id]` | Export as .parquet and .html |

### Built-in Scenarios

| Scenario | Description |
|----------|-------------|
| `memory-leak` | Gradual memory growth to trigger pressure/OOM |
| `oom-kill` | Rapid allocation to trigger OOM killer |
| `crash-loop` | Container crash and restart cycles |
| `sigpipe-crash` | SIGPIPE-induced crash from broken pipe |
| `todo-app` | Multi-service app (frontend, backend, postgres) |

### Exporting Results

Export captured metrics for offline analysis or sharing:

```bash
# Export latest scenario run
./scenario.py export

# Export specific run with custom filename
./scenario.py export a1b2c3d4 -o investigation
# Creates: investigation.parquet, investigation.html
```

The exported HTML is **self-contained**—open it in any browser, works completely offline with the full viewer UI. Share it with teammates without any server setup.

## Adding Scenarios with Gensim

FGM integrates with [gensim](https://github.com/DataDog/gensim) to import realistic multi-service application blueprints. Gensim blueprints define complete microservice architectures that can be compiled into Kubernetes manifests.

> **Note**: FGM currently uses the [`sopell/k8s-adapter`](https://github.com/DataDog/gensim/tree/sopell/k8s-adapter) branch which adds the k8s-adapter for generating Kubernetes manifests from blueprints.

### Import Workflow

```bash
# List available blueprints from gensim
./scenario.py import --list

# Import a blueprint (creates scenario + disruption variants)
./scenario.py import todo-app-redis

# Update gensim cache to get latest blueprints
./scenario.py import --update
```

### What Import Creates

When you import a blueprint like `todo-app-redis`, FGM generates:

1. **Base scenario**: `scenarios/todo-app-redis/` — the healthy application
2. **Disruption variants**: One scenario per disruption defined in the blueprint
   - `todo-app-redis-memory-exhaustion/`
   - `todo-app-redis-cpu-starvation/`
   - `todo-app-redis-network-high-latency/`
   - etc.

Each variant pre-configures a specific failure mode, making it easy to study how different issues manifest in metrics.

### Creating New Blueprints

To add new scenarios:

1. Create a blueprint in [gensim](https://github.com/DataDog/gensim/tree/sopell/k8s-adapter/blueprints)
2. Define the service architecture in `<name>.spec.yaml`
3. Add disruption variants in `<name>.disruption-*.yaml`
4. Import into FGM: `./scenario.py import <name>`

See existing blueprints like [`todo-app-redis`](https://github.com/DataDog/gensim/tree/sopell/k8s-adapter/blueprints/todo-app-redis) for examples.

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

## Plans

Future enhancements ready for implementation:

- **[Correlation Discovery](plans/idea-correlation-discovery.md)** - Automatically discover relationships between metrics across containers with top-K ranked results, lag detection, and cross-container analysis
