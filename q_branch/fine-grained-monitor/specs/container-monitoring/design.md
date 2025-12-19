# Container Monitoring - Technical Design

## Architecture Overview

The fine-grained-monitor is a Rust binary designed to run as a Kubernetes
DaemonSet. It captures detailed resource metrics from all containers on a node
and writes them to a Parquet file for post-hoc analysis.

```text
┌─────────────────────────────────────────────────────────────────────┐
│                      fine-grained-monitor                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────────────┐       ┌─────────────────────────────────┐  │
│  │ Container Discovery │       │         Observer                │  │
│  │   (REQ-FM-001)      │──────▶│   (REQ-FM-002, REQ-FM-003)      │  │
│  │                     │       │                                 │  │
│  │ Scan cgroup fs for  │       │ Vendored from lading:           │  │
│  │ kubepods patterns   │       │ - procfs (PSS, memory, CPU)     │  │
│  └─────────────────────┘       │ - cgroup v2 (mem, CPU, PSI)     │  │
│                                │                                 │  │
│                                │ Emits via metrics-rs macros     │  │
│                                └──────────────┬──────────────────┘  │
│                                               │                     │
│                                               ▼                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    lading_capture (dependency)               │   │
│  │                    (REQ-FM-004, REQ-FM-005)                   │   │
│  │                                                              │   │
│  │  CaptureRecorder ──▶ Accumulator ──▶ Parquet Writer          │   │
│  │  (metrics::Recorder)  (60-tick window)  (Arrow + ZSTD)       │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                               │                     │
│                                               ▼                     │
│                                      metrics.parquet                │
└─────────────────────────────────────────────────────────────────────┘
```

## Dependency Strategy

### External Dependencies (Cargo.toml)

| Crate | Purpose | REQ |
|-------|---------|-----|
| `lading_capture` | Metrics capture, accumulation, Parquet output | REQ-FM-004, REQ-FM-005 |
| `lading_signal` | Shutdown/lifecycle signaling for CaptureManager | REQ-FM-004 |
| `metrics` | Standard metrics facade (gauge!, counter!) | All |
| `tokio` | Async runtime | All |
| `tracing` | Structured logging | All |

### Vendored Code (from lading)

The observer subsystem from lading will be vendored and modified because:
1. The `Sampler` struct is `pub(crate)` and not publicly exposed
2. We need to modify container discovery (multi-container vs single PID)
3. Labels need customization for container/pod identification

Vendored modules:
- `observer/linux/procfs.rs` - Process memory and CPU from /proc
- `observer/linux/procfs/stat.rs` - CPU delta calculation
- `observer/linux/procfs/memory/smaps.rs` - Per-region memory breakdown
- `observer/linux/procfs/memory/smaps_rollup.rs` - Aggregated memory
- `observer/linux/cgroup/` - Cgroup v2 metrics (CPU, memory, IO, PSI)
- `observer/linux/utils/process_descendents.rs` - Process tree walking

## Component Design

### REQ-FM-001: Container Discovery

Container discovery scans the cgroup filesystem without external dependencies.

**Cgroup Path Patterns (containerd/KIND on cgroup v2)**:
```text
/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/
├── kubelet-kubepods-besteffort.slice/
│   └── kubelet-kubepods-besteffort-pod<UID>.slice/
│       └── cri-containerd-<CONTAINER_ID>.scope/
│           ├── cgroup.procs          # PIDs in this container
│           ├── memory.current
│           ├── memory.stat
│           └── cpu.stat
├── kubelet-kubepods-burstable.slice/
│   └── kubelet-kubepods-burstable-pod<UID>.slice/
│       └── cri-containerd-<CONTAINER_ID>.scope/
│           └── ...
└── kubelet-kubepods-pod<UID>.slice/       # Guaranteed QoS (no qos subdirectory)
    └── cri-containerd-<CONTAINER_ID>.scope/
        └── ...
```

**Note**: Pod UIDs in cgroup paths use underscores instead of dashes
(e.g., `pod29b83755_78d3_4345_9a8f_d3017edb5da3` from `29b83755-78d3-4345-9a8f-d3017edb5da3`).

**Discovery Algorithm**:
1. Walk `/sys/fs/cgroup/` recursively
2. Match directories against pattern: `cri-containerd-*.scope` or similar CRI patterns
3. Extract container ID from directory name
4. Read PIDs from `cgroup.procs` file
5. Store mapping: `container_id -> (cgroup_path, Vec<pid>)`

**Refresh Strategy**:
- Re-scan cgroup filesystem each sampling interval
- Compare against previous scan to detect new/removed containers
- Log container lifecycle events for debugging

**Data Model**:
```rust
struct Container {
    id: String,           // Container ID from cgroup path
    cgroup_path: PathBuf, // Full path to cgroup directory
    pids: Vec<i32>,       // PIDs from cgroup.procs
    labels: Labels,       // pod_name, namespace, etc. (future: from cgroup labels)
}
```

### REQ-FM-002: Memory Metrics

**Per-Process Metrics** (from `/proc/<pid>/status` and `/proc/<pid>/smaps_rollup`):
- `pss` - Proportional Set Size (primary metric, accounts for shared pages)
- `pss_anon` - Anonymous PSS
- `pss_file` - File-backed PSS
- `pss_shmem` - Shared memory PSS
- `vmdata`, `vmstk`, `vmexe`, `vmlib` - Segment sizes

PSS is preferred over RSS because it accurately reflects the true memory cost of
a process by dividing shared pages proportionally among all processes sharing
them. This prevents over-counting when multiple processes share libraries.

**Per-Region Metrics** (from `/proc/<pid>/smaps`, requires `--verbose-perf-risk`):
- Aggregated by pathname (mapped file or `[heap]`, `[stack]`, `[anon]`)
- Fields: `pss`, `swap`, `private_clean`, `private_dirty`, `shared_clean`, `shared_dirty`
- Reading smaps acquires the kernel mm lock; disabled by default to avoid
  impacting monitored processes

**Cgroup Metrics** (from `/sys/fs/cgroup/<path>/`):
- `memory.current` - Current usage
- `memory.max` - Limit
- `memory.peak` - High watermark
- `memory.stat` - Detailed breakdown (file, anon, slab, etc.)
- `memory.pressure` - PSI (Pressure Stall Information)

**Sampling Cadence**:
- Basic metrics (PSS, cgroup current): Every sample (1 Hz default)
- Detailed smaps (when enabled): Every 10th sample (0.1 Hz) to reduce overhead

### REQ-FM-003: CPU Metrics

**Per-Process Metrics** (from `/proc/<pid>/stat`):
- `utime`, `stime` - User and system CPU ticks
- Calculated: `cpu_percentage`, `cpu_millicores` (delta-based)

**Cgroup Metrics** (from `/sys/fs/cgroup/<path>/`):
- `cpu.stat` - `usage_usec`, `user_usec`, `system_usec`
- `cpu.max` - CPU limit (for calculating allowed cores)
- `cpu.pressure` - PSI

**Delta Calculation**:
```rust
struct CpuSampler {
    prev_usage: u64,
    prev_instant: Instant,
}

impl CpuSampler {
    fn sample(&mut self, current_usage: u64) -> f64 {
        let delta_usage = current_usage - self.prev_usage;
        let delta_time = self.prev_instant.elapsed();
        let percentage = (delta_usage as f64) / (delta_time.as_micros() as f64) * 100.0;
        self.prev_usage = current_usage;
        self.prev_instant = Instant::now();
        percentage
    }
}
```

### REQ-FM-004: Parquet Output with File Rotation

**Rotation Strategy**:

`lading_capture`'s `CaptureManager` writes to a single Parquet file and only
writes the footer on `close()`. Once closed, the writer cannot be reopened.
To enable users to copy and analyze files without waiting for shutdown, we
implement time-based file rotation:

1. Each rotation interval (default 90 seconds), close the current file and open
   a new one
2. Closing writes the Parquet footer, making the file immediately readable
3. Track total bytes written across all files; shutdown when exceeding 1 GiB

The 90-second rotation interval exceeds the 60-second accumulator window,
ensuring each file contains complete time slices without data spanning files.

**Directory Structure and File Naming**:

```text
/data/
├── session.json                                         # Run manifest
├── dt=2025-12-19/
│   ├── identifier=fine-grained-monitor-abc123/
│   │   ├── metrics-20251219T173000Z.parquet
│   │   ├── metrics-20251219T173130Z.parquet
│   │   └── metrics-20251219T173300Z.parquet
│   └── identifier=fine-grained-monitor-xyz789/
│       └── metrics-20251219T173015Z.parquet
└── dt=2025-12-20/
    └── ...
```

The partitioning scheme uses Hive-style naming (`key=value/`) for compatibility
with Iceberg, Delta, Hudi, and query engines like DuckDB and Spark.

**Identifier** is derived from (in order of preference):
1. `POD_NAME` environment variable (set via Kubernetes downward API)
2. `NODE_NAME` environment variable
3. System hostname

**Session Manifest**:

On startup, write `session.json` to the output directory root:

```json
{
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "identifier": "fine-grained-monitor-abc123",
  "start_time": "2025-12-19T17:30:00Z",
  "config": {
    "sampling_interval_ms": 1000,
    "rotation_seconds": 90,
    "compression_level": 3,
    "verbose_perf_risk": false
  },
  "node_name": "gadget-dev-worker",
  "cluster_name": "gadget-dev",
  "git_rev": "abc1234"
}
```

This preserves run context for debugging sessions weeks later.

**Standardized Labels**:

Every metric includes these labels when available:

| Label | Source | Purpose |
|-------|--------|---------|
| `node_name` | NODE_NAME env / hostname | Node identification |
| `namespace` | cgroup path parsing | Kubernetes namespace |
| `pod_name` | cgroup path parsing | Pod identification |
| `pod_uid` | cgroup path parsing | Immutable pod reference |
| `container_id` | cgroup path parsing | Container identification |
| `container_name` | future: CRI query | Human-readable name |
| `qos_class` | cgroup path parsing | guaranteed/burstable/besteffort |

These labels serve as join keys for cross-container analysis and enable
efficient filtering in queries.

**Rotation Implementation**:

Since `CaptureManager::start()` runs until shutdown and owns the writer, we
cannot directly rotate from outside. Instead, we implement rotation by:

1. Running `CaptureManager` with a short-lived scope
2. Using `shutdown_broadcaster.signal()` to trigger graceful close
3. Creating a new `CaptureManager` for the next file
4. Repeating until total size limit or external shutdown

```rust
async fn run_with_rotation(args: Args) -> anyhow::Result<()> {
    let rotation_interval = Duration::from_secs(args.rotation_seconds);
    let mut total_bytes: u64 = 0;
    let identifier = get_unique_identifier();

    // Write session manifest on startup
    write_session_manifest(&args, &identifier).await?;

    loop {
        let date = chrono::Utc::now().format("%Y-%m-%d");
        let timestamp = chrono::Utc::now().format("%Y%m%dT%H%M%SZ");

        // Create partitioned directory structure
        let partition_dir = args.output_dir
            .join(format!("dt={}", date))
            .join(format!("identifier={}", identifier));
        tokio::fs::create_dir_all(&partition_dir).await?;

        let filename = format!("metrics-{}.parquet", timestamp);
        let output_path = partition_dir.join(&filename);

        // Create new CaptureManager for this rotation period
        let (shutdown_watcher, shutdown_broadcaster) = lading_signal::signal();
        // ... create capture_manager ...

        // Run until rotation interval or external signal
        tokio::select! {
            _ = capture_manager.start() => { /* shutdown received */ }
            _ = tokio::time::sleep(rotation_interval) => {
                shutdown_broadcaster.signal();  // Trigger graceful close
            }
            _ = external_shutdown.recv() => {
                shutdown_broadcaster.signal();
                break;  // Exit rotation loop
            }
        }

        // Check file size and accumulate
        if let Ok(metadata) = tokio::fs::metadata(&output_path).await {
            total_bytes += metadata.len();
        }

        if total_bytes >= MAX_TOTAL_BYTES {
            tracing::warn!(total_bytes, "Total size limit reached");
            break;
        }
    }
    Ok(())
}
```

**Parquet Schema** (from lading_capture):

| Column | Type | Description |
|--------|------|-------------|
| `run_id` | String | UUID for this monitor run |
| `time` | Timestamp(ms) | Wall-clock time |
| `fetch_index` | UInt64 | Tick number (seconds since start) |
| `metric_name` | String | e.g., `status.pss_bytes` |
| `metric_kind` | String | `counter`, `gauge`, or `histogram` |
| `value_int` | UInt64 | Integer value (nullable) |
| `value_float` | Float64 | Float value (nullable) |
| `labels` | Map<String,String> | Standardized labels (see table above) |

**Size Limit**:
- Track cumulative size of all rotated files
- When total exceeds 1 GiB, log warning and stop rotation loop
- Individual files are typically small (90 seconds of data each)

### REQ-FM-005: Late Metric Ingestion

**Accumulator Design** (from lading_capture):
- 60-tick rolling window (60 seconds at 1 Hz)
- Metrics can be written to past ticks within the window
- Each tick advances, oldest tick is flushed to Parquet

**Future Integration Point**:
When Agent output interception is implemented, intercepted metrics will:
1. Parse timestamp from the metric payload
2. Calculate tick offset: `(metric_timestamp - start_time) / tick_duration`
3. Write to accumulator at the calculated tick (if within 60-tick window)

This requirement is implemented by using `lading_capture` as-is. The
`HISTORICAL_SENDER` channel and accumulator handle late metrics automatically.

## Error Handling Strategy

**Container Discovery Errors**:
- Permission denied reading cgroup: Log warning, skip container
- Malformed cgroup path: Log warning, skip
- Empty cgroup.procs: Normal (container may be stopping), skip

**Metric Collection Errors**:
- Process disappeared mid-sample: Expected during container churn, continue
- File read errors: Log at debug level, continue with partial data
- Cgroup read errors: Log warning, emit zero/null for that metric

**Parquet Write Errors**:
- Disk full: Log error, attempt graceful shutdown
- IO errors: Propagate to main, exit with error code

## Configuration

Command-line arguments:
```bash
fine-grained-monitor \
  --output-dir /data \
  --interval-ms 1000 \
  --rotation-seconds 90 \
  --compression-level 3 \
  --verbose-perf-risk    # Optional: enable smaps collection (takes mm lock)
```

Environment variables (typically set via Kubernetes downward API):
- `POD_NAME` - Pod name for identifier and labels
- `NODE_NAME` - Node name for labels
- `CLUSTER_NAME` - Cluster name for labels

## File Structure

```text
fine-grained-monitor/
├── Cargo.toml
├── specs/
│   └── container-monitoring/
│       ├── requirements.md
│       ├── design.md
│       └── executive.md
└── src/
    ├── main.rs              # CLI, lifecycle orchestration
    ├── lib.rs
    ├── discovery.rs         # REQ-FM-001: Cgroup scanning
    └── observer/            # Vendored from lading
        ├── mod.rs
        ├── procfs.rs        # REQ-FM-002, REQ-FM-003
        ├── cgroup.rs        # REQ-FM-002, REQ-FM-003
        └── sampler.rs       # Orchestrates procfs + cgroup
```
