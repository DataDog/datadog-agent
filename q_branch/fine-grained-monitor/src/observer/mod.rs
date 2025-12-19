//! Container resource observation (REQ-FM-002, REQ-FM-003)
//!
//! Uses lading's observer subsystem to collect detailed memory (PSS-focused)
//! and CPU metrics from procfs and cgroups.
//!
//! Note: The actual implementation is Linux-only since it relies on procfs
//! and cgroups. On non-Linux platforms, sampling is a no-op.

use crate::discovery::Container;

#[cfg(target_os = "linux")]
use lading::observer::linux::cgroup::v2 as cgroup_v2;
#[cfg(target_os = "linux")]
use lading::observer::linux::procfs::memory::smaps_rollup;
#[cfg(target_os = "linux")]
use metrics::gauge;
#[cfg(target_os = "linux")]
use rustc_hash::FxHashMap;

/// Per-container sampler state for CPU delta calculations
#[cfg(target_os = "linux")]
#[derive(Debug)]
struct ContainerState {
    cpu_sampler: cgroup_v2::cpu::Sampler,
}

#[cfg(target_os = "linux")]
impl ContainerState {
    fn new() -> Self {
        Self {
            cpu_sampler: cgroup_v2::cpu::Sampler::new(),
        }
    }
}

/// Observer that samples metrics for multiple containers
#[cfg(target_os = "linux")]
#[derive(Debug, Default)]
pub struct Observer {
    /// Per-container state for delta calculations
    container_states: FxHashMap<String, ContainerState>,
}

/// Observer that samples metrics for multiple containers (stub on non-Linux)
#[cfg(not(target_os = "linux"))]
#[derive(Debug, Default)]
pub struct Observer;

#[cfg(target_os = "linux")]
impl Observer {
    pub fn new() -> Self {
        Self {
            container_states: FxHashMap::default(),
        }
    }

    /// Sample all metrics for discovered containers
    ///
    /// Emits metrics via the `metrics` crate (gauge!, counter!) which
    /// routes through CaptureRecorder to the Parquet writer.
    ///
    /// When `verbose_perf_risk` is true, also reads /proc/<pid>/smaps for
    /// per-region memory breakdown. This is disabled by default because
    /// reading smaps acquires the kernel mm lock.
    pub async fn sample(&mut self, containers: &[Container], _verbose_perf_risk: bool) {
        // Clean up state for containers that no longer exist
        let current_ids: std::collections::HashSet<_> =
            containers.iter().map(|c| c.id.clone()).collect();
        self.container_states
            .retain(|id, _| current_ids.contains(id));

        // Sample each container
        for container in containers {
            // Ensure we have state for this container
            let state = self
                .container_states
                .entry(container.id.clone())
                .or_insert_with(ContainerState::new);

            // Build labels for this container
            let labels: Vec<(String, String)> = vec![
                ("container_id".to_string(), container.id.clone()),
                (
                    "pod_uid".to_string(),
                    container.pod_uid.clone().unwrap_or_default(),
                ),
                ("qos_class".to_string(), format!("{:?}", container.qos_class)),
            ];

            // Emit basic container metrics
            gauge!("container.pid_count", &labels).set(container.pids.len() as f64);

            // Sample cgroup v2 metrics (memory.stat, memory.current, etc.)
            if let Err(e) = cgroup_v2::poll(&container.cgroup_path, &labels).await {
                tracing::debug!(
                    container_id = %container.id,
                    error = %e,
                    "Failed to poll cgroup v2 metrics"
                );
            }

            // Sample cgroup CPU metrics with delta calculation
            if let Err(e) = state
                .cpu_sampler
                .poll(&container.cgroup_path, &labels)
                .await
            {
                tracing::debug!(
                    container_id = %container.id,
                    error = %e,
                    "Failed to poll cgroup CPU metrics"
                );
            }

            // Sample procfs metrics for each PID in the container
            // Use static labels for smaps_rollup (it expects &[(&'static str, String)])
            for &pid in &container.pids {
                let pid_labels: [(&'static str, String); 4] = [
                    ("container_id", container.id.clone()),
                    (
                        "pod_uid",
                        container.pod_uid.clone().unwrap_or_default(),
                    ),
                    ("qos_class", format!("{:?}", container.qos_class)),
                    ("pid", pid.to_string()),
                ];

                // Read PSS metrics from smaps_rollup
                let mut aggr = smaps_rollup::Aggregator::default();
                if let Err(e) = smaps_rollup::poll(pid, &pid_labels, &mut aggr).await {
                    tracing::trace!(
                        container_id = %container.id,
                        pid = pid,
                        error = %e,
                        "Failed to poll smaps_rollup (process may have exited)"
                    );
                }
            }

            // TODO: If verbose_perf_risk is enabled, also read full smaps
            // for per-region memory breakdown. This is gated because it
            // acquires the kernel mm lock.
        }
    }
}

#[cfg(not(target_os = "linux"))]
impl Observer {
    pub fn new() -> Self {
        Self
    }

    /// No-op sampling on non-Linux platforms
    pub async fn sample(&mut self, containers: &[Container], _verbose_perf_risk: bool) {
        let _ = containers;
        tracing::warn!(
            "Observer sampling is only available on Linux - metrics will not be collected"
        );
    }
}
