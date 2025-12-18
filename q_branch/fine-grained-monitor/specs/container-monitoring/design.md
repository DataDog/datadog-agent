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

**Cgroup Path Patterns (containerd/KIND)**:
```text
/sys/fs/cgroup/kubepods.slice/
├── kubepods-burstable.slice/
│   └── kubepods-burstable-pod<UID>.slice/
│       └── cri-containerd-<CONTAINER_ID>.scope/
│           ├── cgroup.procs          # PIDs in this container
│           ├── memory.current
│           └── cpu.stat
└── kubepods-besteffort.slice/
    └── ...
```

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

### REQ-FM-004: Parquet Output

**Integration with lading_capture**:

```rust
// Lifecycle setup
let (shutdown_watcher, shutdown_broadcaster) = lading_signal::signal();
let (experiment_watcher, _) = lading_signal::signal();
let (target_watcher, target_broadcaster) = lading_signal::signal();

let mut capture_manager = CaptureManager::new_parquet(
    PathBuf::from("/data/metrics.parquet"),
    flush_seconds,      // How often to flush to disk
    compression_level,  // ZSTD compression (1-22)
    shutdown_watcher,
    experiment_watcher,
    target_watcher,
    Duration::from_secs(60),  // Accumulator expiration
).await?;

// Add global labels
capture_manager.add_global_label("node", node_name);
capture_manager.add_global_label("cluster", cluster_name);

// Start capture (installs global metrics recorder)
target_broadcaster.signal();  // Marks time-zero

// Run observer loop in parallel with capture manager
tokio::select! {
    _ = observer_loop() => {},
    _ = capture_manager.start() => {},
}

// Graceful shutdown
shutdown_broadcaster.signal();
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
| `labels` | Map<String,String> | Container ID, pod name, etc. |

**Size Limit**:
- Monitor file size after each flush
- When file exceeds 1 GiB, log warning and initiate graceful shutdown
- Prevents runaway disk usage during long collection sessions

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

```yaml
# config.yaml (future)
sampling_interval_ms: 1000    # REQ-FM-003
output_path: /data/metrics.parquet
compression_level: 3          # ZSTD level
smaps_sample_divisor: 10      # Sample smaps every N ticks
```

Initial implementation will use command-line arguments:
```bash
fine-grained-monitor \
  --output /data/metrics.parquet \
  --interval-ms 1000 \
  --compression-level 3 \
  --verbose-perf-risk    # Optional: enable smaps collection (takes mm lock)
```

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
