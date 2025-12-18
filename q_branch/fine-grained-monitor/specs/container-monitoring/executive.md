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
container IDs and PIDs without kubelet or CRI dependencies. Observer code is
vendored from `lading` and modified for multi-container support, emitting
metrics via the standard `metrics` crate facade.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-FM-001:** Discover Running Containers | ❌ Not Started | Cgroup scan implementation pending |
| **REQ-FM-002:** View Detailed Memory Usage | ❌ Not Started | Vendor procfs/smaps parsers from lading |
| **REQ-FM-003:** View Detailed CPU Usage | ❌ Not Started | Vendor procfs/cgroup CPU parsers from lading |
| **REQ-FM-004:** Analyze Data Post-Hoc | ✅ Complete | `lading_capture` integration with CaptureManager, Parquet output, graceful shutdown, 1 GiB file size limit |
| **REQ-FM-005:** Capture Delayed Metrics | ⏭️ Planned | Accumulator support via `lading_capture`; active use scheduled for later phase when Agent output interception is implemented |

**Progress:** 1 of 5 complete

## Implementation Notes

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
