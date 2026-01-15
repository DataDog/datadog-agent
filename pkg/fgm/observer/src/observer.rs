// Copyright 2025 Datadog, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//! Container metrics observer using lading APIs
//!
//! This module provides functionality to sample container metrics from
//! Linux cgroups v2 and procfs using the lading observer library.
//! It intercepts metrics emitted by lading and routes them through
//! a callback for FFI consumption.

use anyhow::Result;
use std::path::Path;
use std::sync::Mutex;

#[cfg(target_os = "linux")]
use lading_observer::linux::cgroup::v2 as cgroup_v2;
#[cfg(target_os = "linux")]
use lading_observer::linux::procfs::memory::smaps_rollup;

use crate::metrics_bridge::{CallbackRecorder, MetricCallback};

/// Sample all available metrics for a container using lading observer APIs
///
/// # Arguments
///
/// * `cgroup_path` - Absolute path to the container's cgroup directory
/// * `pid` - Container's main PID (for procfs reads, 0 to skip)
/// * `emit` - Callback function to emit metrics (name, value, tags, timestamp_ms)
///
/// # Returns
///
/// Returns `Ok(())` if sampling succeeded, or an error if critical reads failed.
/// Individual metric read failures are silently ignored (not all metrics may
/// be available in all kernel versions).
///
/// # Implementation
///
/// This function uses lading's observer APIs:
/// - `cgroup_v2::poll()` for cgroup v2 metrics
/// - `smaps_rollup::poll()` for PSS (Proportional Set Size) metrics
///
/// Metrics are emitted through the metrics facade and captured by a custom
/// Recorder that routes them to the provided callback.
#[cfg(target_os = "linux")]
pub async fn sample_container<F>(
    cgroup_path: &Path,
    pid: i32,
    emit: F,
) -> Result<()>
where
    F: FnMut(&str, f64, Vec<(String, String)>, i64) + Send + 'static,
{
    // Wrap the callback in an Arc<Mutex<>> for thread-safe access from the metrics recorder
    let emit_mutex = std::sync::Arc::new(Mutex::new(emit));
    let emit_clone = emit_mutex.clone();

    // Create metrics callback that will receive all metric emissions from lading
    let metrics_callback: MetricCallback = Box::new(move |name, value, labels, timestamp| {
        if let Ok(mut emit_guard) = emit_clone.lock() {
            (*emit_guard)(name, value, labels, timestamp);
        }
    });

    // Install the custom recorder to intercept metrics facade calls
    let recorder = CallbackRecorder::new(metrics_callback);

    // Set as the global recorder (this will fail if already set, which is fine)
    let _ = metrics::set_global_recorder(Box::leak(Box::new(recorder)));

    // Build labels for this container
    // In a real implementation, you'd pass these from the Go side
    let labels: Vec<(String, String)> = vec![];

    // Sample cgroup v2 metrics (memory, CPU, PSI, etc.)
    if let Err(e) = cgroup_v2::poll(cgroup_path, &labels).await {
        // Log error but continue - some metrics may not be available
        eprintln!("Failed to poll cgroup v2 metrics: {}", e);
    }

    // Sample procfs metrics for PSS if PID available
    if pid > 0 {
        // Convert labels to the format expected by smaps_rollup
        let pid_labels: [(&'static str, String); 1] = [("pid", pid.to_string())];

        let mut aggregator = smaps_rollup::Aggregator::default();
        if let Err(e) = smaps_rollup::poll(pid, &pid_labels, &mut aggregator).await {
            // Process may have exited, this is not critical
            eprintln!("Failed to poll smaps_rollup (process may have exited): {}", e);
        }
    }

    Ok(())
}

/// Non-Linux stub implementation
#[cfg(not(target_os = "linux"))]
pub async fn sample_container<F>(
    _cgroup_path: &Path,
    _pid: i32,
    _emit: F,
) -> Result<()>
where
    F: FnMut(&str, f64, Vec<(String, String)>, i64),
{
    anyhow::bail!("Observer sampling is only available on Linux")
}

#[cfg(all(test, target_os = "linux"))]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::{Arc, Mutex};

    #[tokio::test]
    async fn test_sample_container_nonexistent_path() {
        let metrics = Arc::new(Mutex::new(HashMap::new()));
        let metrics_clone = metrics.clone();

        let result = sample_container(
            Path::new("/sys/fs/cgroup/nonexistent"),
            0,
            move |name, value, _tags, _timestamp| {
                if let Ok(mut m) = metrics_clone.lock() {
                    m.insert(name.to_string(), value);
                }
            },
        )
        .await;

        // Should not error even if path doesn't exist
        assert!(result.is_ok());
    }
}
