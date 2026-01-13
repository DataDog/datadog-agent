# Fine-Grained Monitor

Capture 1Hz container metrics without touching the Agent you're debugging.

When investigating CPU throttling, memory pressure, or mysterious resource spikes in the Datadog Agent, you need visibility into what's happening—but the Agent's own telemetry might be part of the problem. This monitor runs independently, writing high-resolution metrics to Parquet files you can analyze after the fact.

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
