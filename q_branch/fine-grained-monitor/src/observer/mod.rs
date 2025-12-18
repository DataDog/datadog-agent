//! Container resource observation (REQ-FM-002, REQ-FM-003)
//!
//! Vendored and modified from lading's observer subsystem.
//! Collects detailed memory (PSS-focused) and CPU metrics from procfs and cgroups.

// TODO: Vendor from lading:
// - procfs.rs (process memory and CPU from /proc/<pid>/)
// - cgroup.rs (cgroup v2 metrics)
// - stat.rs (CPU delta calculation)
// - smaps.rs (per-region memory breakdown, gated by verbose_perf_risk)

use crate::discovery::Container;

/// Sample all metrics for a container
///
/// Emits metrics via the `metrics` crate (gauge!, counter!) which
/// routes through CaptureRecorder to the Parquet writer.
///
/// When `verbose_perf_risk` is true, also reads /proc/<pid>/smaps for
/// per-region memory breakdown. This is disabled by default because
/// reading smaps acquires the kernel mm lock.
pub fn sample(_container: &Container, _verbose_perf_risk: bool) {
    // TODO: Implement REQ-FM-002 and REQ-FM-003
    // 1. For each PID in container.pids:
    //    a. Read /proc/<pid>/smaps_rollup for PSS metrics (primary)
    //    b. Read /proc/<pid>/stat for CPU metrics
    //    c. If verbose_perf_risk: read /proc/<pid>/smaps for per-region breakdown
    // 2. Read cgroup metrics from container.cgroup_path:
    //    a. memory.current, memory.stat, memory.pressure
    //    b. cpu.stat, cpu.pressure
    // 3. Emit all metrics with container labels
}
