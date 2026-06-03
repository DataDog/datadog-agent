// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Minimal architecture and process selection for the fake NVML demo library.

use std::sync::OnceLock;

const DEFAULT_DEVICE_COUNT: usize = 1;
const DEFAULT_PROCESS_PID: u32 = 1001;

/// Fields used by the fake NVML library to describe the single demo architecture.
#[derive(Debug, Clone)]
pub struct Architecture {
    pub nvml_architecture: u32,
    pub device_name: String,
    pub cuda_compute_major: i32,
    pub cuda_compute_minor: i32,
    pub num_gpu_cores: u32,
    /// Total device memory, in bytes.
    pub total_memory_bytes: u64,
    pub supports_gpm: bool,
}

/// Returns the hard-coded demo architecture. This intentionally avoids the full
/// architecture/spec matrix; the demo only needs one plausible GPU.
pub fn current_arch() -> &'static Architecture {
    static CURRENT: OnceLock<Architecture> = OnceLock::new();
    CURRENT.get_or_init(|| Architecture {
        nvml_architecture: 9, // NVML_DEVICE_ARCH_HOPPER
        device_name: "NVIDIA H100 80GB HBM3 (fake)".to_string(),
        cuda_compute_major: 9,
        cuda_compute_minor: 0,
        num_gpu_cores: 16_384,
        total_memory_bytes: 80 * 1024 * 1024 * 1024,
        supports_gpm: true,
    })
}

/// Returns the number of fake devices to expose, controlled by
/// `FAKE_NVML_DEVICE_COUNT` (1..=16, default 1).
pub fn device_count() -> usize {
    static COUNT: OnceLock<usize> = OnceLock::new();
    *COUNT.get_or_init(|| {
        std::env::var("FAKE_NVML_DEVICE_COUNT")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .map(|n| n.clamp(1, 16))
            .unwrap_or(DEFAULT_DEVICE_COUNT)
    })
}

/// Returns the fake GPU process PID for a device. Set `FAKE_NVML_PROCESS_PID`
/// to a real host PID, e.g. a demo container's init process, so the Agent's
/// existing PID-to-container path can attach container/job tags to GPU process
/// metrics.
pub fn fake_process_pid(device_index: usize) -> u32 {
    static PID: OnceLock<Option<u32>> = OnceLock::new();
    let base = PID
        .get_or_init(|| {
            std::env::var("FAKE_NVML_PROCESS_PID")
                .ok()
                .and_then(|v| v.trim().parse::<u32>().ok())
                .filter(|pid| *pid > 0)
        })
        .unwrap_or(DEFAULT_PROCESS_PID);

    base.saturating_add(device_index as u32)
}
