# Fine-Grained Monitor - System Design

## What It Does

Fine-grained-monitor (fgm) captures detailed container resource metrics at
1-second resolution and makes them available for both human exploration and
LLM-assisted analysis. It runs as a Kubernetes DaemonSet, collecting metrics
from every container on each node and storing them in Parquet files for
efficient querying.

The system answers questions like:
- "Which container is using the most memory on this node?"
- "Does this container's CPU usage show periodic spikes?"
- "When did this container's memory usage change significantly?"

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Kubernetes Cluster                                 │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                    Namespace: fine-grained-monitor                      │ │
│  │                                                                         │ │
│  │  ┌──────────────────────────────────────────────────────────────────┐  │ │
│  │  │                    DaemonSet (one pod per node)                   │  │ │
│  │  │                                                                   │  │ │
│  │  │   ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐        │  │ │
│  │  │   │     fgm     │   │ fgm-viewer  │   │ fgm-consolidator│        │  │ │
│  │  │   │  collector  │   │   web UI    │   │   (optional)    │        │  │ │
│  │  │   │             │   │  :8050      │   │                 │        │  │ │
│  │  │   └──────┬──────┘   └──────┬──────┘   └────────┬────────┘        │  │ │
│  │  │          │ write           │ read              │ read/write      │  │ │
│  │  │          └─────────────────┼───────────────────┘                 │  │ │
│  │  │                            ▼                                      │  │ │
│  │  │                    /data volume (hostPath)                        │  │ │
│  │  │                    ├── session.json                               │  │ │
│  │  │                    └── dt=YYYY-MM-DD/identifier=pod-xyz/          │  │ │
│  │  │                        ├── metrics-*.parquet                      │  │ │
│  │  │                        └── metrics-*.containers (sidecar)         │  │ │
│  │  └──────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                         │ │
│  │  ┌─────────────────────┐                                                │ │
│  │  │   fgm-mcp-server    │◄──── Claude Code / AI Agents                   │ │
│  │  │    Deployment       │      (via port-forward or direct)              │ │
│  │  │    :8080            │                                                │ │
│  │  └──────────┬──────────┘                                                │ │
│  │             │ routes queries to correct node's viewer                   │ │
│  │             ▼                                                           │ │
│  │      [ viewer :8050 on Node A ]  [ viewer :8050 on Node B ]  ...        │ │
│  │                                                                         │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

All fgm components run in the `fine-grained-monitor` namespace. This enables:
- Simple cleanup (`kubectl delete namespace fine-grained-monitor`)
- Least-privilege RBAC (namespace-scoped Role instead of ClusterRole)
- Clear resource ownership (`kubectl get all -n fine-grained-monitor`)

The system has four binaries, all built from the same Rust codebase:

| Binary | Deployment | Purpose |
|--------|------------|---------|
| `fgm` | DaemonSet container | Collects metrics, writes Parquet files |
| `fgm-viewer` | DaemonSet sidecar | Serves web UI and HTTP API |
| `fgm-consolidator` | DaemonSet sidecar (optional) | Merges small Parquet files |
| `fgm-mcp-server` | Deployment (1 replica) | Routes LLM queries to correct node |

## Data Flow

### Stage 1: Container Discovery

The collector discovers containers by scanning the cgroup filesystem every
second. No external dependencies (CRI, kubelet API) are required.

```
/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/
├── kubelet-kubepods-besteffort.slice/
│   └── kubelet-kubepods-besteffort-pod<UID>.slice/
│       └── cri-containerd-<CONTAINER_ID>.scope/
│           ├── cgroup.procs     ← PIDs in container
│           ├── memory.current
│           └── cpu.stat
├── kubelet-kubepods-burstable.slice/
│   └── ...
└── kubelet-kubepods-pod<UID>.slice/    (Guaranteed QoS)
    └── ...
```

The collector extracts container IDs from directory names, reads PIDs from
`cgroup.procs`, and infers QoS class from the path structure. Pod metadata
(name, namespace, labels) is enriched via Kubernetes API when available.

### Stage 2: Metric Collection

For each discovered container, the collector samples:

**Memory metrics** (from `/proc/<pid>/smaps_rollup` and cgroup):
- PSS (Proportional Set Size) - true memory cost accounting for shared pages
- PSS breakdown: anonymous, file-backed, shared memory
- Cgroup memory: current, peak, limit

**CPU metrics** (from `/proc/<pid>/stat` and cgroup):
- User and system CPU time (converted to millicores)
- Cgroup CPU usage with delta calculation for percentages

All metric collection is delegated to the `lading` crate's observer subsystem,
which handles the low-level procfs and cgroup parsing.

### Stage 3: Parquet Storage

Metrics are written to Parquet files using `lading_capture`, which provides:

- **Accumulator window**: Metrics buffer for 60 seconds before flushing
- **File rotation**: New file every 90 seconds (ensures readable files while collector runs)
- **Compression**: ZSTD level 3 for ~10x compression
- **Partitioning**: Hive-style `dt=YYYY-MM-DD/identifier=<pod-name>/` structure

**Parquet schema:**

| Column | Type | Description |
|--------|------|-------------|
| `time` | Timestamp(ms) | Wall-clock sample time |
| `metric_name` | String | e.g., `cgroup.v2.cpu.stat.usage_usec` |
| `value` | Float64 | Metric value |
| `l_container_id` | String | Container ID (with bloom filter) |
| `l_pod_name` | String | Pod name |
| `l_namespace` | String | Kubernetes namespace |
| `l_qos_class` | String | Guaranteed/Burstable/BestEffort |
| `l_node_name` | String | Node name |

Labels use `l_*` prefix columns rather than a Map type, enabling bloom filters
on high-cardinality fields like `l_container_id` for efficient point lookups.

Each rotated file is immediately readable (Parquet footer written on close),
enabling real-time analysis without waiting for collector shutdown.

### Stage 4: Container Sidecar Files

Each parquet file gets a companion `.containers` sidecar file that enables
fast container discovery without decompressing parquet data:

**File naming:**
- Parquet: `metrics-20260112T215249Z.parquet`
- Sidecar: `metrics-20260112T215249Z.containers`

**Format:** Binary (bincode-serialized) for 10-100x faster serialization than JSON

**Structure:**
```rust
struct ContainerSidecar {
    version: u8,  // Current: v2
    containers: Vec<SidecarContainer>,
}

struct SidecarContainer {
    container_id: String,
    pod_name: Option<String>,
    container_name: Option<String>,
    namespace: Option<String>,
    pod_uid: Option<String>,
    qos_class: String,  // "Guaranteed" | "Burstable" | "BestEffort"
    labels: Option<HashMap<String, String>>,
}
```

**Session manifest:** A `session.json` file at `/data/` root provides run metadata:
```json
{
  "run_id": "abc123",
  "identifier": "fine-grained-monitor-xyz",
  "start_time": "2026-01-12T21:52:49Z",
  "node_name": "worker-1",
  "cluster_name": "prod-us-east",
  "sampling_interval_ms": 1000,
  "rotation_seconds": 90
}
```

**Performance benefit:** Viewer startup reads tiny sidecar files (~2-5KB each)
instead of scanning parquet row groups (700-800ms per file). This enables
sub-second container discovery across thousands of files.

### Stage 5: File Consolidation (Optional)

With 90-second rotation, a day of collection produces ~960 small files. The
optional `fgm-consolidator` sidecar periodically merges old files:

- Scans for files older than 5 minutes
- Groups by partition (date + identifier)
- Streams through files, writing consolidated output
- Atomic rename ensures no data loss on interruption

This keeps file counts manageable for long-running deployments while preserving
all data.

### Stage 6: Interactive Viewing

The `fgm-viewer` sidecar serves a web UI on port 8050:

```
┌─────────────────────────────────────────────────────────────────────┐
│  Metric: [cgroup.v2.cpu.stat.usage_usec ▼]                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Panel 1: cpu_usage                                     [Edit] [✕]  │
│    ┌────────────────────────────────────────────────────────────┐   │
│    │  ▄▄    ▄▄    ▄▄    ▄▄    ▄▄    ▄▄    ▄▄    ▄▄              │   │
│    │▄████▄████▄████▄████▄████▄████▄████▄████▄                   │   │
│    └────────────────────────────────────────────────────────────┘   │
│    • pod-frontend / cpu_usage                                       │
│    • pod-backend / cpu_usage                                        │
│                                                                     │
│  Panel 2: memory_current                                [Edit] [✕]  │
│    ┌────────────────────────────────────────────────────────────┐   │
│    │────────────────────────────────────────────────────────────│   │
│    └────────────────────────────────────────────────────────────┘   │
│    • pod-frontend / memory_current                                  │
│                                                                     │
│  [═══════════════●═══] Range Overview                               │
└─────────────────────────────────────────────────────────────────────┘
```

**Key features:**
- Up to 5 synchronized chart panels for comparing metrics
- Container search with fuzzy matching
- Zoom/pan with synchronized time axis across panels
- Studies: periodicity detection and changepoint detection

**Studies** are analytical overlays that detect patterns:

| Study | Algorithm | Use Case |
|-------|-----------|----------|
| Periodicity | Sliding-window autocorrelation | Find recurring spikes (cron jobs, GC cycles) |
| Changepoint | Bayesian Online Detection (BOCPD) | Find sudden shifts (deployments, load changes) |

Studies return structured findings (timestamps, confidence scores, magnitudes)
rather than raw statistics, making them suitable for both human review and
programmatic consumption.

### Stage 7: LLM Agent Access

The `fgm-mcp-server` Deployment enables LLM agents (Claude Code, AI SRE agents)
to query metrics programmatically via the Model Context Protocol (MCP).

**Why a separate server?** Each DaemonSet pod only has metrics for its own
node. The MCP server discovers all viewer pods via Kubernetes API and routes
queries to the correct node based on a required `node` parameter.

**MCP tools exposed:**

| Tool | Purpose |
|------|---------|
| `list_nodes` | Discover available nodes and their readiness status |
| `list_metrics` | Get available metric names and study types |
| `list_containers` | Find containers with filters (namespace, QoS, name prefix) |
| `analyze_container` | Run a study on a container's metrics |

**Design principle:** No raw timeseries data is returned to agents. LLMs cannot
meaningfully interpret thousands of data points. Instead, agents receive:
- Summary statistics (avg, max, min, stddev, trend classification)
- Study findings (detected periods, changepoints with timestamps and magnitudes)

This enables agents to answer questions like "is this container's memory usage
increasing?" without drowning in numbers.

## Key Technical Decisions

### Why Parquet?

Parquet provides columnar storage with excellent compression for timeseries
data. A day of metrics from a busy node compresses to ~50-100 MB. The format
supports predicate pushdown, enabling the viewer to load only relevant time
ranges without scanning entire files.

### Why Sidecar Architecture?

Running viewer and consolidator as sidecars in the DaemonSet pod:
- Shares the data volume without network transfer
- Lifecycle tied to collector (no orphaned viewers)
- Per-node isolation (each viewer only sees its node's data)

### Why Separate MCP Server?

The MCP server runs as a Deployment (not in the DaemonSet) because:
- Agents need a single stable endpoint, not per-node access
- Node-to-pod routing requires cluster-wide pod discovery
- SSE connections benefit from a stable pod (no DaemonSet scheduling churn)

### Why 90-Second Rotation?

The `lading_capture` accumulator flushes every 60 seconds. Rotating at 90
seconds ensures each file contains at least one complete flush cycle, avoiding
partial data at file boundaries.

## Local Development

The system can also run locally for development and debugging:

```bash
# Point viewer at a parquet file or directory
fgm-viewer /path/to/metrics.parquet --port 8050

# Or a directory of parquet files
fgm-viewer /path/to/data/ --port 8050
```

This enables analyzing exported data without a Kubernetes cluster.

## Component Specifications

Detailed implementation specifications for each component:

| Component | Specification |
|-----------|---------------|
| Collector | `specs/container-monitoring/design.md` |
| Viewer | `specs/metrics-viewer/design.md` |
| Consolidator | `specs/streaming-consolidator/design.md` |
| MCP Server | `specs/mcp-metrics-viewer/design.md` |
