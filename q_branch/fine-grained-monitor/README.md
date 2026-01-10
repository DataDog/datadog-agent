# Fine-Grained Monitor

Capture 1Hz container metrics without touching the Agent you're debugging.

When you're investigating CPU throttling, memory pressure, or mysterious resource spikes in the Datadog Agent, you need visibility into what's happening—but the Agent's own telemetry might be part of the problem. This monitor runs independently, writing high-resolution metrics to Parquet files you can analyze after the fact.

## Quick Start

**Build and deploy to gadget-dev cluster:**

```bash
cd q_branch/fine-grained-monitor

# Build the image
docker build -t fine-grained-monitor:rotation .

# Load into Kind cluster
docker save fine-grained-monitor:rotation | limactl shell gadget-k8s-host -- docker load
limactl shell gadget-k8s-host -- kind load docker-image fine-grained-monitor:rotation --name gadget-dev

# Deploy as DaemonSet
kubectl apply -f deploy/daemonset.yaml --context kind-gadget-dev
```

**Collect data:**

```bash
# Watch it run
kubectl logs -f -l app=fine-grained-monitor --context kind-gadget-dev

# Copy parquet files from node
kubectl cp <pod>:/data ./collected-metrics --context kind-gadget-dev
```

**Visualize:**

More realistic multi-service applications generated via k8s-adapter from Gensim:

| Scenario | Components | Purpose |
|----------|------------|---------|
| `todo-app` | frontend, backend, postgres | Realistic app with service interactions |

These scenarios exercise complex behaviors that emerge from service interactions rather than isolated faults.

### Exporting Scenario Results

Export captured metrics as a self-contained HTML file for offline analysis or sharing:

```bash
# Run a scenario and let it collect data
./scenario.py run memory-leak
# Wait for interesting behavior (~5-10 minutes for memory-leak)

# Export to self-contained HTML
./scenario.py export <run_id>
# Creates: scenario-results-<run_id>.html (~280KB-5MB depending on data)

# Or specify output path
./scenario.py export <run_id> --output my-analysis.html
```

The exported HTML includes:
- **All captured metrics** embedded as Parquet (loaded via parquet-wasm)
- **Full viewer UI** with theme switching, panel management, container selection
- **Works offline** - no server required, just open in any modern browser

Use cases:
- Share findings with teammates without cluster access
- Archive scenario results for future reference
- Present analysis in environments without network access

## Validation Paths

The same captured data validates both human and LLM analysis:

**Human validation (fgm-viewer):**
```bash
./dev.py start --data ./collected-metrics/some-file.parquet
# Opens browser to interactive viewer with pan/zoom, periodicity detection
```

## What It Captures

Per-container, every second:
- **CPU**: usage in millicores, user/system split, throttling stats
- **Memory**: current usage, PSS, cgroup limits and pressure (PSI)
- **Metadata**: pod name, namespace, QoS class, container ID

Output: Parquet files partitioned by date and pod, queryable with DuckDB/pandas/Spark.

## Viewer Features

The `fgm-viewer` binary serves an interactive web UI:
- Metric selection with fuzzy search
- Container filtering by namespace/QoS class
- Drag-to-zoom, scroll-wheel zoom, overview navigator
- **Periodicity Study**: autocorrelation-based detection of recurring patterns (throttling cycles, GC, cron jobs)

## Project Structure

```
src/
├── main.rs              # Monitor binary (runs on nodes)
├── bin/fgm-viewer.rs    # Viewer binary (runs locally)
├── metrics_viewer/      # Web UI and analysis
└── observer/            # Metrics collection logic

deploy/
└── daemonset.yaml       # K8s deployment manifest

specs/                   # Requirements and design docs
```

## Benchmarks

Performance benchmarks for the parquet query codepath using [divan](https://github.com/nvzqz/divan).

**Generate test data:**

```bash
# Small dataset (quick iteration)
cargo run --release --bin generate-bench-data -- --scenario small

# Medium dataset (realistic testing)
cargo run --release --bin generate-bench-data -- --scenario medium
```

| Scenario | Files | Containers | Metrics | Rows/File |
|----------|-------|------------|---------|-----------|
| small | 2 | 10 | 5 | 10K |
| medium | 50 | 50 | 30 | 50K |
| large | 200 | 100 | 30 | 100K |
| production | 500 | 100 | 30 | 100K |

**Run benchmarks:**

```bash
# Run all benchmarks (suppress verbose PERF logging)
cargo bench 2>/dev/null

# Run specific benchmark
cargo bench -- scan_metadata 2>/dev/null

# Use different dataset
BENCH_DATA=testdata/bench/medium cargo bench 2>/dev/null
```

Benchmarks measure:
- `scan_metadata` - Startup time to build metadata index
- `get_timeseries_single_container` - Single container timeseries query
- `get_timeseries_all_containers` - All containers timeseries query
