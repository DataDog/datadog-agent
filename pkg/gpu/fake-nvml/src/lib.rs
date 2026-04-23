// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Fake NVML shared library for development and integration testing.
//!
//! This library implements the NVML C ABI and returns realistic static data
//! for two fake NVIDIA H100 GPUs. It is intended to allow the Datadog Agent's
//! GPU check to run on machines without real NVIDIA hardware.
//!
//! # Activation
//!
//! The agent never loads this library automatically. You must explicitly set
//! `gpu.nvml_lib_path` in `datadog.yaml` to activate it:
//!
//! ```yaml
//! gpu:
//!   enabled: true
//!   nvml_lib_path: "/opt/datadog-agent/embedded/dev/libnvidia-ml-fake.so.1"
//! ```
//!
//! # Safety
//!
//! All exported `extern "C"` functions:
//! - Validate pointer arguments before dereferencing
//! - Return `NVML_ERROR_INVALID_ARGUMENT` on null or out-of-range inputs
//! - Wrap their bodies in `std::panic::catch_unwind` to prevent panics from
//!   crossing the C ABI boundary

#![allow(non_camel_case_types, non_snake_case)]

use core::ffi::{c_char, c_int, c_uint, c_void};
use std::panic;
use std::sync::OnceLock;

mod arch;
use arch::{current_arch, device_count};

// ---------------------------------------------------------------------------
// NVML return codes
// ---------------------------------------------------------------------------

pub type NvmlReturn = u32;

pub const NVML_SUCCESS: NvmlReturn = 0;
pub const NVML_ERROR_INVALID_ARGUMENT: NvmlReturn = 1;
pub const NVML_ERROR_NOT_SUPPORTED: NvmlReturn = 3;

// ---------------------------------------------------------------------------
// NVML opaque device handle
//
// We encode the device index (0-based) as (index + 1) cast to *mut c_void.
// This avoids a null handle for index 0 while keeping the encoding trivial.
// ---------------------------------------------------------------------------

pub type NvmlDevice = *mut c_void;

fn device_from_index(idx: usize) -> NvmlDevice {
    (idx + 1) as *mut c_void
}

fn index_from_device(dev: NvmlDevice) -> Option<usize> {
    let raw = dev as usize;
    if raw == 0 || raw > devices().len() {
        return None;
    }
    Some(raw - 1)
}

// ---------------------------------------------------------------------------
// NVML struct definitions — layouts must match the real NVML C headers exactly
// ---------------------------------------------------------------------------

/// Matches `nvmlMemory_t` from nvml.h
#[repr(C)]
pub struct NvmlMemory {
    pub total: u64,
    pub free: u64,
    pub used: u64,
}

/// Matches `nvmlMemory_v2_t` from nvml.h (struct-versioning pattern)
#[repr(C)]
pub struct NvmlMemoryV2 {
    pub version: u32,
    pub total: u64,
    pub reserved: u64,
    pub free: u64,
    pub used: u64,
}

/// Matches `nvmlUtilization_t` from nvml.h
#[repr(C)]
pub struct NvmlUtilization {
    pub gpu: u32,    // % GPU utilization
    pub memory: u32, // % memory bandwidth utilization
}

/// Matches `nvmlProcessInfo_t` from nvml.h
#[repr(C)]
pub struct NvmlProcessInfo {
    pub pid: u32,
    pub used_gpu_memory: u64,
    pub gpu_instance_id: u32,
    pub compute_instance_id: u32,
}

// ---------------------------------------------------------------------------
// Per-device fake state
//
// Identity fields (uuid, name, cores, compute capability, architecture, total
// memory) are derived at first-access time from the GPU architecture spec
// selected via `FAKE_NVML_ARCH`. All other fields are intentionally plausible
// constants — the point is to exercise the agent's metric plumbing, not to
// model a specific SKU exactly.
// ---------------------------------------------------------------------------

struct FakeDevice {
    uuid: String,
    name: String,
    cores: u32,
    compute_major: i32,
    compute_minor: i32,
    /// NVML_DEVICE_ARCH_* constant reported by nvmlDeviceGetArchitecture().
    architecture: u32,
    total_mem_bytes: u64,
    free_mem_bytes: u64,
    reserved_mem_bytes: u64,
    temperature_c: u32,
    power_usage_mw: u32,
    power_limit_mw: u32,
    clock_sm_mhz: u32,
    clock_mem_mhz: u32,
    clock_graphics_mhz: u32,
    clock_video_mhz: u32,
    /// Fake PID of a process running on this device
    fake_pid: u32,
    /// Memory used by the fake process, in bytes
    fake_pid_mem_bytes: u64,
    fan_speed_pct: u32,
    performance_state: u32,
    bar1_total_bytes: u64,
    bar1_free_bytes: u64,
    energy_mj: u64,
}

const GIB: u64 = 1024 * 1024 * 1024;

/// Build `FakeDevice` number `idx` (0-based) for the currently selected
/// architecture. Identity values come from the spec; other fields are
/// architecture-agnostic placeholders.
fn make_device(idx: usize) -> FakeDevice {
    let arch = current_arch();
    let total = arch.total_memory_bytes;
    // Plausible memory-usage shape: half-full with 1 GiB reserved.
    let reserved = GIB.min(total / 8);
    let free = (total - reserved) / 2;

    // UUIDs must be stable and distinct per index; encode the index in the
    // last block so scraped metrics can tell devices apart.
    let uuid = format!(
        "GPU-{idx:08x}-FAKE-{arch:04x}-{arch:04x}-{idx:012x}",
        idx = idx,
        // Use the NVML arch constant for a deterministic per-arch block.
        arch = arch.nvml_architecture as u16
    );

    FakeDevice {
        uuid,
        name: arch.device_name.clone(),
        cores: arch.num_gpu_cores,
        compute_major: arch.cuda_compute_major,
        compute_minor: arch.cuda_compute_minor,
        architecture: arch.nvml_architecture,
        total_mem_bytes: total,
        free_mem_bytes: free,
        reserved_mem_bytes: reserved,
        temperature_c: 60 + (idx as u32 % 8),
        power_usage_mw: 280_000,
        power_limit_mw: 400_000,
        clock_sm_mhz: 1980,
        clock_mem_mhz: 2619,
        clock_graphics_mhz: 1980,
        clock_video_mhz: 1530,
        fake_pid: 1001 + idx as u32,
        fake_pid_mem_bytes: 4 * GIB,
        fan_speed_pct: 38 + (idx as u32 % 8),
        performance_state: 0,
        bar1_total_bytes: 64 * GIB,
        bar1_free_bytes: 60 * GIB,
        energy_mj: 100_000_000 + (idx as u64 * 1_000_000),
    }
}

fn devices() -> &'static [FakeDevice] {
    static DEVICES: OnceLock<Vec<FakeDevice>> = OnceLock::new();
    DEVICES.get_or_init(|| (0..device_count()).map(make_device).collect())
}

// ---------------------------------------------------------------------------
// Helper: copy a Rust string into a C char buffer as a null-terminated string.
//
// Caller does not need to include a trailing '\0' in `src`. Returns
// NVML_ERROR_INVALID_ARGUMENT if `buf` is null or `len` is 0. Silently
// truncates to `len-1` + null if `src` is too long.
// ---------------------------------------------------------------------------

fn copy_str_to_buf(src: &str, buf: *mut c_char, len: u32) -> NvmlReturn {
    if buf.is_null() || len == 0 {
        return NVML_ERROR_INVALID_ARGUMENT;
    }
    let src_bytes = src.as_bytes();
    // Stop at the first interior NUL so we never copy past it.
    let effective_len = src_bytes
        .iter()
        .position(|&b| b == 0)
        .unwrap_or(src_bytes.len());
    let copy_len = effective_len.min(len as usize - 1);
    // SAFETY: We've checked buf is non-null and len > 0; copy_len <= len-1.
    unsafe {
        std::ptr::copy_nonoverlapping(src_bytes.as_ptr() as *const c_char, buf, copy_len);
        *buf.add(copy_len) = 0;
    }
    NVML_SUCCESS
}

// ---------------------------------------------------------------------------
// Macro: wrap an exported function body in catch_unwind, returning
// NVML_ERROR_NOT_SUPPORTED if a panic occurs. This prevents undefined
// behaviour from a Rust panic unwinding across the C ABI boundary.
// ---------------------------------------------------------------------------

macro_rules! ffi_guard {
    ($body:block) => {{
        match panic::catch_unwind(panic::AssertUnwindSafe(|| $body)) {
            Ok(ret) => ret,
            Err(_) => NVML_ERROR_NOT_SUPPORTED,
        }
    }};
}

// ---------------------------------------------------------------------------
// Exported NVML symbols
// ---------------------------------------------------------------------------

// --- Lifecycle ---

#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlInit_v2() -> NvmlReturn {
    ffi_guard!({ NVML_SUCCESS })
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlShutdown() -> NvmlReturn {
    ffi_guard!({ NVML_SUCCESS })
}

// --- System ---

/// `nvmlSystemGetDriverVersion(char *version, unsigned int length)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlSystemGetDriverVersion(buf: *mut c_char, len: c_uint) -> NvmlReturn {
    ffi_guard!({ copy_str_to_buf("535.154.05\0", buf, len) })
}

// --- Device enumeration ---

/// `nvmlDeviceGetCount(unsigned int *count)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetCount(count: *mut c_uint) -> NvmlReturn {
    ffi_guard!({
        if count.is_null() {
            return NVML_ERROR_INVALID_ARGUMENT;
        }
        unsafe { *count = devices().len() as c_uint };
        NVML_SUCCESS
    })
}

/// `nvmlDeviceGetHandleByIndex(unsigned int index, nvmlDevice_t *device)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetHandleByIndex(
    idx: c_uint,
    device: *mut NvmlDevice,
) -> NvmlReturn {
    ffi_guard!({
        if device.is_null() {
            return NVML_ERROR_INVALID_ARGUMENT;
        }
        if idx as usize >= devices().len() {
            return NVML_ERROR_INVALID_ARGUMENT;
        }
        unsafe { *device = device_from_index(idx as usize) };
        NVML_SUCCESS
    })
}

/// `nvmlDeviceGetIndex(nvmlDevice_t device, unsigned int *index)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetIndex(device: NvmlDevice, idx: *mut c_uint) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if idx.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *idx = i as c_uint };
                NVML_SUCCESS
            }
        }
    })
}

// --- Device identity ---

/// `nvmlDeviceGetUUID(nvmlDevice_t device, char *uuid, unsigned int length)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetUUID(
    device: NvmlDevice,
    buf: *mut c_char,
    len: c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => copy_str_to_buf(&devices()[i].uuid, buf, len),
        }
    })
}

/// `nvmlDeviceGetName(nvmlDevice_t device, char *name, unsigned int length)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetName(
    device: NvmlDevice,
    buf: *mut c_char,
    len: c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => copy_str_to_buf(&devices()[i].name, buf, len),
        }
    })
}

/// `nvmlDeviceGetNumGpuCores(nvmlDevice_t device, unsigned int *numCores)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetNumGpuCores(
    device: NvmlDevice,
    cores: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if cores.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *cores = devices()[i].cores };
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetCudaComputeCapability(nvmlDevice_t device, int *major, int *minor)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetCudaComputeCapability(
    device: NvmlDevice,
    major: *mut c_int,
    minor: *mut c_int,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if major.is_null() || minor.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe {
                    *major = devices()[i].compute_major;
                    *minor = devices()[i].compute_minor;
                }
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetArchitecture(nvmlDevice_t device, nvmlDeviceArchitecture_t *arch)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetArchitecture(
    device: NvmlDevice,
    arch: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if arch.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *arch = devices()[i].architecture };
                NVML_SUCCESS
            }
        }
    })
}

// --- Memory ---

/// `nvmlDeviceGetMemoryInfo(nvmlDevice_t device, nvmlMemory_t *memory)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetMemoryInfo(
    device: NvmlDevice,
    mem: *mut NvmlMemory,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if mem.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe {
                    (*mem).total = devices()[i].total_mem_bytes;
                    (*mem).free = devices()[i].free_mem_bytes;
                    (*mem).used =
                        devices()[i].total_mem_bytes - devices()[i].free_mem_bytes - devices()[i].reserved_mem_bytes;
                }
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetMemoryInfo_v2(nvmlDevice_t device, nvmlMemory_v2_t *memory)`
///
/// go-nvml checks for this symbol via LookupSymbol; providing it enables
/// the higher-priority memory metrics path in the stateless collector.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetMemoryInfo_v2(
    device: NvmlDevice,
    mem: *mut NvmlMemoryV2,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if mem.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                // Preserve the caller-supplied version field; only populate data fields.
                unsafe {
                    (*mem).total = devices()[i].total_mem_bytes;
                    (*mem).reserved = devices()[i].reserved_mem_bytes;
                    (*mem).free = devices()[i].free_mem_bytes;
                    (*mem).used =
                        devices()[i].total_mem_bytes - devices()[i].free_mem_bytes - devices()[i].reserved_mem_bytes;
                }
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetBAR1MemoryInfo(nvmlDevice_t device, nvmlBAR1Memory_t *bar1Memory)`
///
/// `nvmlBAR1Memory_t` has the same layout as `nvmlMemory_t`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetBAR1MemoryInfo(
    device: NvmlDevice,
    bar1mem: *mut NvmlMemory,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if bar1mem.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe {
                    (*bar1mem).total = devices()[i].bar1_total_bytes;
                    (*bar1mem).free = devices()[i].bar1_free_bytes;
                    (*bar1mem).used = devices()[i].bar1_total_bytes - devices()[i].bar1_free_bytes;
                }
                NVML_SUCCESS
            }
        }
    })
}

// --- Thermal and power ---

/// `nvmlDeviceGetTemperature(nvmlDevice_t device, nvmlTemperatureSensors_t sensorType, unsigned int *temp)`
///
/// sensorType 0 = NVML_TEMPERATURE_GPU (the only defined sensor type).
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetTemperature(
    device: NvmlDevice,
    _sensor_type: c_uint,
    temp: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if temp.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *temp = devices()[i].temperature_c };
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetPowerUsage(nvmlDevice_t device, unsigned int *power)` — milliwatts
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetPowerUsage(
    device: NvmlDevice,
    power: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if power.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *power = devices()[i].power_usage_mw };
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetPowerManagementLimit(nvmlDevice_t device, unsigned int *limit)` — milliwatts
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetPowerManagementLimit(
    device: NvmlDevice,
    limit: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if limit.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *limit = devices()[i].power_limit_mw };
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetTotalEnergyConsumption(nvmlDevice_t device, unsigned long long *energy)` — millijoules
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetTotalEnergyConsumption(
    device: NvmlDevice,
    energy: *mut u64,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if energy.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *energy = devices()[i].energy_mj };
                NVML_SUCCESS
            }
        }
    })
}

// --- Clocks ---

// Clock type constants (nvmlClockType_t)
const NVML_CLOCK_GRAPHICS: u32 = 0;
const NVML_CLOCK_SM: u32 = 1;
const NVML_CLOCK_MEM: u32 = 2;
const NVML_CLOCK_VIDEO: u32 = 3;

fn clock_for_type(dev: &FakeDevice, clock_type: u32) -> Option<u32> {
    match clock_type {
        NVML_CLOCK_GRAPHICS => Some(dev.clock_graphics_mhz),
        NVML_CLOCK_SM => Some(dev.clock_sm_mhz),
        NVML_CLOCK_MEM => Some(dev.clock_mem_mhz),
        NVML_CLOCK_VIDEO => Some(dev.clock_video_mhz),
        _ => None,
    }
}

/// `nvmlDeviceGetClockInfo(nvmlDevice_t device, nvmlClockType_t type, unsigned int *clock)` — MHz
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetClockInfo(
    device: NvmlDevice,
    clock_type: c_uint,
    clock: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if clock.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                match clock_for_type(&devices()[i], clock_type) {
                    None => NVML_ERROR_INVALID_ARGUMENT,
                    Some(v) => {
                        unsafe { *clock = v };
                        NVML_SUCCESS
                    }
                }
            }
        }
    })
}

/// `nvmlDeviceGetMaxClockInfo(nvmlDevice_t device, nvmlClockType_t type, unsigned int *clock)` — MHz
///
/// Returns the same values as `nvmlDeviceGetClockInfo` (fake devices always
/// run at their "boost" clocks).
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetMaxClockInfo(
    device: NvmlDevice,
    clock_type: c_uint,
    clock: *mut c_uint,
) -> NvmlReturn {
    // Delegate to GetClockInfo — same values for a fake device.
    unsafe { nvmlDeviceGetClockInfo(device, clock_type, clock) }
}

// --- Utilization ---

/// `nvmlDeviceGetUtilizationRates(nvmlDevice_t device, nvmlUtilization_t *utilization)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetUtilizationRates(
    device: NvmlDevice,
    util: *mut NvmlUtilization,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(_i) => {
                if util.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe {
                    (*util).gpu = 42;
                    (*util).memory = 30;
                }
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetPerformanceState(nvmlDevice_t device, nvmlPstates_t *pState)`
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetPerformanceState(
    device: NvmlDevice,
    pstate: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if pstate.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *pstate = devices()[i].performance_state };
                NVML_SUCCESS
            }
        }
    })
}

/// `nvmlDeviceGetFanSpeed(nvmlDevice_t device, unsigned int *speed)` — percent
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetFanSpeed(
    device: NvmlDevice,
    speed: *mut c_uint,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if speed.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe { *speed = devices()[i].fan_speed_pct };
                NVML_SUCCESS
            }
        }
    })
}

// --- Processes ---

/// `nvmlDeviceGetComputeRunningProcesses(nvmlDevice_t device, unsigned int *infoCount, nvmlProcessInfo_t *infos)`
///
/// Returns one fake process per device. The caller first calls with `infos=NULL`
/// to query the count, then again with a buffer. Both patterns are handled.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nvmlDeviceGetComputeRunningProcesses(
    device: NvmlDevice,
    info_count: *mut c_uint,
    infos: *mut NvmlProcessInfo,
) -> NvmlReturn {
    ffi_guard!({
        match index_from_device(device) {
            None => NVML_ERROR_INVALID_ARGUMENT,
            Some(i) => {
                if info_count.is_null() {
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                // Caller is querying the count
                if infos.is_null() {
                    unsafe { *info_count = 1 };
                    return NVML_SUCCESS;
                }
                // Caller provided a buffer — fill one entry
                if unsafe { *info_count } < 1 {
                    // Buffer too small; tell caller how many we need
                    unsafe { *info_count = 1 };
                    return NVML_ERROR_INVALID_ARGUMENT;
                }
                unsafe {
                    (*infos).pid = devices()[i].fake_pid;
                    (*infos).used_gpu_memory = devices()[i].fake_pid_mem_bytes;
                    (*infos).gpu_instance_id = u32::MAX; // NVML_NO_GI_ID
                    (*infos).compute_instance_id = u32::MAX; // NVML_NO_CI_ID
                    *info_count = 1;
                }
                NVML_SUCCESS
            }
        }
    })
}
