// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Architecture selection for the fake NVML shared library.
//!
//! The canonical list of supported GPU architectures and their representative
//! device-identity values lives in `pkg/collector/corechecks/gpu/spec/architectures.yaml`
//! — the same file consumed by the agent's GPU check. This module embeds that
//! file at build time via `include_str!` and lets callers pick a single
//! architecture at process start via the `FAKE_NVML_ARCH` environment variable.
//!
//! All devices exported by the fake library share the selected architecture
//! ("homogeneous node"). This intentionally mirrors real-world deployments
//! where a host exposes N identical GPUs.

use std::sync::OnceLock;

use serde::Deserialize;

// ---------------------------------------------------------------------------
// YAML schema types
//
// These mirror the subset of `ArchitectureSpec` / `ArchitectureDefaults` from
// `pkg/collector/corechecks/gpu/spec/spec.go` that the fake library needs.
// Extra YAML fields (e.g. `unsupported_fields_by_device_mode`) are silently
// ignored by serde_yaml; capability flags we care about are read directly.
// ---------------------------------------------------------------------------

#[derive(Debug, Deserialize)]
struct SpecRoot {
    architectures: std::collections::BTreeMap<String, RawArchSpec>,
}

#[derive(Debug, Deserialize)]
struct RawArchSpec {
    #[serde(default)]
    capabilities: RawCapabilities,
    #[serde(default)]
    defaults: RawDefaults,
}

#[derive(Debug, Default, Deserialize)]
struct RawCapabilities {
    #[serde(default)]
    gpm: bool,
}

#[derive(Debug, Default, Deserialize)]
struct RawDefaults {
    #[serde(default)]
    nvml_architecture: u32,
    #[serde(default)]
    device_name: String,
    #[serde(default)]
    cuda_compute_major: i32,
    #[serde(default)]
    cuda_compute_minor: i32,
    #[serde(default)]
    num_gpu_cores: u32,
    #[serde(default)]
    total_memory_mib: u64,
}

// ---------------------------------------------------------------------------
// Public architecture descriptor
// ---------------------------------------------------------------------------

/// Fields used by the fake NVML library to describe the selected architecture.
#[derive(Debug, Clone)]
#[allow(dead_code)] // `name` and `supports_gpm` are consumed by the GPM stubs and logging.
pub struct Architecture {
    pub name: String,
    pub nvml_architecture: u32,
    pub device_name: String,
    pub cuda_compute_major: i32,
    pub cuda_compute_minor: i32,
    pub num_gpu_cores: u32,
    /// Total device memory, in bytes.
    pub total_memory_bytes: u64,
    pub supports_gpm: bool,
}

// ---------------------------------------------------------------------------
// Embedded spec
//
// Path is resolved relative to this source file. The BUILD.bazel target
// declares the YAML as `compile_data` so the Bazel sandbox sees it at the
// same relative location.
// ---------------------------------------------------------------------------

const SPEC_YAML: &str = include_str!("../../../collector/corechecks/gpu/spec/architectures.yaml");

const DEFAULT_ARCH: &str = "hopper";
const DEFAULT_DEVICE_COUNT: usize = 2;

fn load_spec() -> SpecRoot {
    // Panics here are desirable: a malformed spec is a build/packaging bug
    // that must be fixed before the fake library is usable. ffi_guard! at
    // call sites still catches the panic before it crosses the C ABI.
    serde_yaml::from_str(SPEC_YAML).expect("fake-nvml: failed to parse embedded architectures.yaml")
}

fn resolve_arch_name() -> String {
    std::env::var("FAKE_NVML_ARCH")
        .ok()
        .map(|v| v.trim().to_lowercase())
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| DEFAULT_ARCH.to_string())
}

fn build_current_arch() -> Architecture {
    let spec = load_spec();
    let requested = resolve_arch_name();
    let raw = spec.architectures.get(&requested).unwrap_or_else(|| {
        eprintln!(
            "fake-nvml: unknown FAKE_NVML_ARCH={:?}; falling back to {:?}. Known archs: {:?}",
            requested,
            DEFAULT_ARCH,
            spec.architectures.keys().collect::<Vec<_>>()
        );
        spec.architectures
            .get(DEFAULT_ARCH)
            .expect("fake-nvml: default architecture missing from embedded spec")
    });

    let name = if spec.architectures.contains_key(&requested) {
        requested
    } else {
        DEFAULT_ARCH.to_string()
    };

    Architecture {
        name,
        nvml_architecture: raw.defaults.nvml_architecture,
        device_name: raw.defaults.device_name.clone(),
        cuda_compute_major: raw.defaults.cuda_compute_major,
        cuda_compute_minor: raw.defaults.cuda_compute_minor,
        num_gpu_cores: raw.defaults.num_gpu_cores,
        total_memory_bytes: raw.defaults.total_memory_mib * 1024 * 1024,
        supports_gpm: raw.capabilities.gpm,
    }
}

/// Returns the architecture selected for this process. Resolved once at first
/// call and cached for the lifetime of the process.
pub fn current_arch() -> &'static Architecture {
    static CURRENT: OnceLock<Architecture> = OnceLock::new();
    CURRENT.get_or_init(build_current_arch)
}

/// Returns the number of fake devices to expose, controlled by
/// `FAKE_NVML_DEVICE_COUNT` (1..=16, default 2).
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn embedded_spec_parses() {
        let spec = load_spec();
        for arch in [
            "pascal",
            "volta",
            "turing",
            "ampere",
            "hopper",
            "ada",
            "blackwell",
        ] {
            let a = spec
                .architectures
                .get(arch)
                .unwrap_or_else(|| panic!("spec missing arch {arch}"));
            assert!(
                a.defaults.nvml_architecture != 0,
                "{arch} missing nvml_architecture"
            );
            assert!(
                !a.defaults.device_name.is_empty(),
                "{arch} missing device_name"
            );
            assert!(a.defaults.num_gpu_cores > 0, "{arch} missing num_gpu_cores");
            assert!(
                a.defaults.total_memory_mib > 0,
                "{arch} missing total_memory_mib"
            );
        }
    }

    #[test]
    fn gpm_flag_matches_spec() {
        let spec = load_spec();
        // From the current architectures.yaml: only hopper and blackwell have gpm=true.
        assert!(spec.architectures["hopper"].capabilities.gpm);
        assert!(spec.architectures["blackwell"].capabilities.gpm);
        assert!(!spec.architectures["pascal"].capabilities.gpm);
        assert!(!spec.architectures["ampere"].capabilities.gpm);
    }
}
