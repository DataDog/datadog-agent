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

All metrics are written to a single Parquet file for post-hoc analysis using
standard tools like DuckDB, pandas, or Spark. Labels identify each metric by
container, pod, and node, enabling filtering and aggregation during analysis.

## Technical Summary

The monitor uses `lading_capture` as a dependency for the metrics pipeline.
`CaptureRecorder` implements the `metrics::Recorder` trait, routing all
`gauge!()` and `counter!()` calls to an in-memory registry. The 60-tick
accumulator windows metrics before flushing to the Parquet writer, which uses
Arrow with ZSTD compression.

Container discovery scans `/sys/fs/cgroup/` for `kubepods` patterns, extracting
container IDs and PIDs without kubelet or CRI dependencies. Observer code uses
`lading`'s observer APIs directly (exposed via local path dependency), emitting
metrics via the standard `metrics` crate facade.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-FM-001:** Discover Running Containers | ✅ Complete | Cgroup v2 filesystem scan with containerd/CRI-O/Docker pattern matching, pod UID extraction, QoS class detection |
| **REQ-FM-002:** View Detailed Memory Usage | ✅ Complete | Uses lading's `smaps_rollup` for PSS metrics per-PID, `cgroup_v2::poll()` for memory.stat/memory.current |
| **REQ-FM-003:** View Detailed CPU Usage | ✅ Complete | Uses lading's `cgroup_v2::cpu::Sampler` for CPU delta calculations with percentage and millicores |
| **REQ-FM-004:** Analyze Data Post-Hoc | ✅ Complete | `lading_capture` integration with CaptureManager, Parquet output, graceful shutdown, 1 GiB file size limit |
| **REQ-FM-005:** Capture Delayed Metrics | ⏭️ Planned | Accumulator support via `lading_capture`; active use scheduled for later phase when Agent output interception is implemented |

**Progress:** 4 of 5 complete

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

The Parquet output pipeline is now functional:

- **CaptureManager** from `lading_capture` handles the metrics-to-Parquet pipeline
- **Three signal pairs** coordinate lifecycle: `shutdown`, `experiment_started`, `target_running`
- **Global labels** (node, cluster) automatically attach to all metrics
- **File size monitoring** triggers graceful shutdown when file exceeds 1 GiB
- **Signal handlers** for SIGINT/SIGTERM ensure clean shutdown with proper Parquet finalization

The observer code can now emit metrics via `gauge!()` and `counter!()` macros, which flow through:
1. `CaptureRecorder` (implements `metrics::Recorder`)
2. In-memory registry with 60-tick accumulator window
3. Parquet writer with ZSTD compression

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
