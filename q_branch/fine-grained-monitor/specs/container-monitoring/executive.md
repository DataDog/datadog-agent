# Container Monitoring - Executive Summary

## Requirements Summary

The fine-grained-monitor solves the "who watches the watcher" problem for
Datadog Agent development. When iterating on the Agent itself, developers cannot
rely on the Agent for observability data. This tool provides an independent,
high-resolution view of container resource consumption.

Users deploy the monitor as a Kubernetes DaemonSet. It automatically discovers
all containers on the node via cgroup filesystem scanning, requiring no manual
configuration. For each container, it captures detailed memory metrics (PSS,
per-region breakdown via optional smaps) and CPU metrics (per-process and
cgroup-level) at a configurable cadence (default 1 Hz).

Metrics are written to Parquet files in a partitioned directory structure
(`dt=YYYY-MM-DD/identifier=<pod-name>/`). Files rotate every 90 seconds,
exceeding the 60-second accumulator window to ensure complete time slices.
A session manifest (`session.json`) captures run configuration and context.
Standardized labels (node_name, namespace, pod_name, pod_uid, container_id,
container_name, qos_class) enable reliable cross-container analysis. Standard
tools like DuckDB, pandas, or Spark can query the partitioned directory.

## Technical Summary

The monitor uses `lading_capture` as a dependency for the metrics pipeline.
`CaptureRecorder` implements the `metrics::Recorder` trait, routing all
`gauge!()` and `counter!()` calls to an in-memory registry. The 60-tick
accumulator windows metrics before flushing to the Parquet writer, which uses
Arrow with ZSTD compression.

Time-based file rotation uses `lading_capture`'s channel-based rotation API.
`CaptureManager::start_with_rotation()` spawns the event loop in a background
task and returns a `RotationSender` immediately. The main loop sends
`RotationRequest` messages every 90 seconds; the CaptureManager creates a new
file, swaps formats via `StateMachine::replace_format()`, and closes the old
file (writing the Parquet footer). Files are written to Hive-style partitioned
directories (`dt=YYYY-MM-DD/identifier=<pod>/`) for Iceberg/Delta/Hudi
compatibility. A session manifest preserves run context for debugging.

Container discovery scans `/sys/fs/cgroup/` for `kubepods` patterns, extracting
container IDs and PIDs without kubelet or CRI dependencies. Observer code uses
`lading`'s observer APIs directly (exposed via git dependency), emitting metrics
via the standard `metrics` crate facade.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-FM-001:** Discover Running Containers | ✅ Complete | Cgroup v2 filesystem scan with containerd/CRI-O/Docker pattern matching, pod UID extraction, QoS class detection |
| **REQ-FM-002:** View Detailed Memory Usage | ✅ Complete | Uses lading's `smaps_rollup` for PSS metrics per-PID, `cgroup_v2::poll()` for memory.stat/memory.current |
| **REQ-FM-003:** View Detailed CPU Usage | ✅ Complete | Uses lading's `cgroup_v2::cpu::Sampler` for CPU delta calculations with percentage and millicores |
| **REQ-FM-004:** Analyze Data Post-Hoc | ✅ Complete | 90s rotation, dt/identifier partitioning, standardized labels, session manifest all implemented and verified |

**Progress:** 4 of 4 complete

## Implementation Notes

### REQ-FM-001 Implementation (Completed)

Container discovery via cgroup filesystem scanning is now functional:

- **Cgroup v2 support** with recursive directory traversal
- **Multiple CRI patterns** matched: `cri-containerd-*.scope`, `crio-*.scope`, `docker-*.scope`
- **Pod UID extraction** from parent cgroup path with underscore-to-dash conversion
- **QoS class detection** inferred from cgroup path hierarchy (BestEffort, Burstable, Guaranteed)
- **Empty container filtering** skips containers with no processes in `cgroup.procs`
- **Test coverage** with tempfile-based integration tests for all patterns

Verified path patterns on KIND cluster:
`/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-{qos}.slice/kubelet-kubepods-{qos}-pod{uid}.slice/cri-containerd-{id}.scope`

### REQ-FM-004 Implementation (Completed)

File rotation with valid Parquet footers is now fully implemented and verified in the
gadget-dev cluster.

**Implementation:**
- **Channel-based rotation** via `lading_capture`'s new `start_with_rotation()` API
- **RotationSender** returned immediately, event loop runs in background task
- **90-second rotation** exceeds 60-second accumulator window for complete time slices
- **Hive-style partitioning** with `dt=YYYY-MM-DD/identifier=<pod-name>/` structure
- **Session manifest** (`session.json`) written on startup with run_id, config, git_rev
- **Global labels** (node_name, cluster_name) added to all metrics via CaptureManager
- **Total size limit** (1 GiB) across all rotated files triggers graceful shutdown

**Rotation Flow:**
1. `CaptureManager::start_with_rotation()` spawns event loop, returns `RotationSender`
2. Main loop sends `RotationRequest` with new file path every 90 seconds
3. CaptureManager creates new file, calls `StateMachine::replace_format()` to swap
4. Old format is closed, writing Parquet footer
5. Response sent back confirming rotation success

**Verified:**
- Rotated files have valid `PAR1` magic at both start and end (footer present)
- Files contain 38,496+ rows of cgroup metrics per 90-second window
- 204 unique metrics captured (CPU, memory, pressure stats)
- Labels include container_id, pod_uid, qos_class, node_name, cluster_name

### REQ-FM-002 Implementation (Completed)

Memory metrics collection uses lading's observer APIs directly:

- **Per-PID PSS** via `smaps_rollup::poll()` reads `/proc/{pid}/smaps_rollup`
- **Cgroup memory stats** via `cgroup_v2::poll()` reads `memory.stat`, `memory.current`
- **Labels** include container_id, pod_uid, qos_class, and pid (for per-process metrics)
- **Error handling** gracefully skips exited processes (common in container churn)
- **Linux-only** implementation with no-op stub for macOS development

### REQ-FM-003 Implementation (Completed)

CPU metrics collection uses lading's observer APIs:

- **CPU delta tracking** via `cgroup_v2::cpu::Sampler` maintains per-container state for calculating CPU percentage
- **Metrics emitted** include total/user/system CPU percentage and millicores
- **Cgroup v2 CPU stats** from `cpu.stat` file
- **Per-container state** stored in `Observer.container_states` HashMap for delta calculations
- **State cleanup** removes entries for containers that no longer exist

