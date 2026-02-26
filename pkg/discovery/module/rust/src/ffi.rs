// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! C ABI interface for `get_services`.
//!
//! Exports two symbols:
//! - `dd_discovery_get_services` — runs discovery and returns a heap-allocated result.
//! - `dd_discovery_free` — deallocates the result.
//!
//! All strings are length-delimited (`dd_str`), not NUL-terminated.
//! The caller must copy any data it needs (e.g. `C.GoStringN`) before calling free.

#![allow(non_camel_case_types)] // C ABI types use C naming conventions

use std::ffi::c_char;
use std::ptr;

use crate::params::Params;
use crate::services::{self, Service, ServicesResponse};
use crate::tracer_metadata::TracerMetadata;
use crate::ust::UST;

// ---------------------------------------------------------------------------
// #[repr(C)] types
// ---------------------------------------------------------------------------

/// Length-delimited byte string — avoids NUL-termination so the Go caller
/// can use `C.GoStringN(data, len)` directly without `strlen`.
/// NULL `data` with `len == 0` represents an absent/empty value.
#[repr(C)]
pub struct dd_str {
    pub data: *const c_char,
    pub len: usize,
}

/// Slice of `u16` values (port numbers).
#[repr(C)]
pub struct dd_u16_slice {
    pub data: *const u16,
    pub len: usize,
}

/// Array of `dd_str`.
#[repr(C)]
pub struct dd_strs {
    pub data: *const dd_str,
    pub len: usize,
}

/// Unified Service Tagging.
#[repr(C)]
pub struct dd_ust {
    pub service: dd_str,
    pub env: dd_str,
    pub version: dd_str,
}

#[repr(C)]
pub struct dd_tracer_metadata {
    pub schema_version: u8,
    pub runtime_id: dd_str,
    pub tracer_language: dd_str,
    pub tracer_version: dd_str,
    pub hostname: dd_str,
    pub service_name: dd_str,
    pub service_env: dd_str,
    pub service_version: dd_str,
}

#[repr(C)]
pub struct dd_tracer_metadata_slice {
    pub data: *const dd_tracer_metadata,
    pub len: usize,
}

#[repr(C)]
pub struct dd_service {
    pub pid: i32,
    pub generated_name: dd_str,
    pub generated_name_source: dd_str,
    pub additional_generated_names: dd_strs,
    pub tracer_metadata: dd_tracer_metadata_slice,
    pub ust: dd_ust,
    pub tcp_ports: dd_u16_slice,
    pub udp_ports: dd_u16_slice,
    pub log_files: dd_strs,
    pub apm_instrumentation: bool,
    pub language: dd_str,
    pub service_type: dd_str,
    pub has_nvidia_gpu: bool,
}

#[repr(C)]
pub struct dd_discovery_result {
    pub services: *mut dd_service,
    pub services_len: usize,
    pub injected_pids: *mut i32,
    pub injected_pids_len: usize,
    pub gpu_pids: *mut i32,
    pub gpu_pids_len: usize,
}

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

impl dd_str {
    /// NULL sentinel for absent values.
    const NULL: Self = Self {
        data: ptr::null(),
        len: 0,
    };

    /// Create a `dd_str` by copying a `&str` into a heap allocation.
    /// Keeps free logic uniform — every non-NULL `dd_str` is heap-owned.
    fn from_str(s: &str) -> Self {
        if s.is_empty() {
            return Self::NULL;
        }
        let boxed = Box::<[u8]>::from(s.as_bytes());
        let len = boxed.len();
        let data = Box::into_raw(boxed) as *const c_char;
        Self { data, len }
    }
}

impl From<String> for dd_str {
    fn from(s: String) -> Self {
        if s.is_empty() {
            return Self::NULL;
        }
        let boxed = s.into_bytes().into_boxed_slice();
        let len = boxed.len();
        let data = Box::into_raw(boxed) as *const c_char;
        Self { data, len }
    }
}

impl From<Option<String>> for dd_str {
    fn from(opt: Option<String>) -> Self {
        match opt {
            Some(s) => dd_str::from(s),
            None => dd_str::NULL,
        }
    }
}

impl From<Vec<String>> for dd_strs {
    fn from(v: Vec<String>) -> Self {
        if v.is_empty() {
            return Self {
                data: ptr::null(),
                len: 0,
            };
        }
        let converted: Vec<dd_str> = v.into_iter().map(dd_str::from).collect();
        let boxed = converted.into_boxed_slice();
        let len = boxed.len();
        let data = Box::into_raw(boxed) as *const dd_str;
        Self { data, len }
    }
}

fn vec_u16_to_slice(v: Option<Vec<u16>>) -> dd_u16_slice {
    match v {
        None => dd_u16_slice {
            data: ptr::null(),
            len: 0,
        },
        Some(v) if v.is_empty() => dd_u16_slice {
            data: ptr::null(),
            len: 0,
        },
        Some(v) => {
            let boxed = v.into_boxed_slice();
            let len = boxed.len();
            let data = Box::into_raw(boxed) as *const u16;
            dd_u16_slice { data, len }
        }
    }
}

impl From<UST> for dd_ust {
    fn from(ust: UST) -> Self {
        Self {
            service: dd_str::from(ust.service),
            env: dd_str::from(ust.env),
            version: dd_str::from(ust.version),
        }
    }
}

impl From<TracerMetadata> for dd_tracer_metadata {
    fn from(tm: TracerMetadata) -> Self {
        Self {
            schema_version: tm.schema_version,
            runtime_id: dd_str::from(tm.runtime_id),
            tracer_language: dd_str::from_str(tm.tracer_language.as_str()),
            tracer_version: dd_str::from(tm.tracer_version),
            hostname: dd_str::from(tm.hostname),
            service_name: dd_str::from(tm.service_name),
            service_env: dd_str::from(tm.service_env),
            service_version: dd_str::from(tm.service_version),
        }
    }
}

fn tracer_metadata_vec(v: Vec<TracerMetadata>) -> dd_tracer_metadata_slice {
    if v.is_empty() {
        return dd_tracer_metadata_slice {
            data: ptr::null(),
            len: 0,
        };
    }
    let converted: Vec<dd_tracer_metadata> = v.into_iter().map(dd_tracer_metadata::from).collect();
    let boxed = converted.into_boxed_slice();
    let len = boxed.len();
    let data = Box::into_raw(boxed) as *const dd_tracer_metadata;
    dd_tracer_metadata_slice { data, len }
}

impl From<Service> for dd_service {
    fn from(svc: Service) -> Self {
        Self {
            pid: svc.pid,
            generated_name: dd_str::from(svc.generated_name),
            generated_name_source: svc
                .generated_name_source
                .map_or(dd_str::NULL, |src| dd_str::from_str(src.as_str())),
            additional_generated_names: dd_strs::from(svc.additional_generated_names),
            tracer_metadata: tracer_metadata_vec(svc.tracer_metadata),
            ust: dd_ust::from(svc.ust),
            tcp_ports: vec_u16_to_slice(svc.tcp_ports),
            udp_ports: vec_u16_to_slice(svc.udp_ports),
            log_files: dd_strs::from(svc.log_files),
            apm_instrumentation: svc.apm_instrumentation,
            language: dd_str::from_str(svc.language.as_str()),
            service_type: dd_str::from(svc.service_type),
            has_nvidia_gpu: svc.has_nvidia_gpu,
        }
    }
}

fn vec_i32_to_raw(v: Vec<i32>) -> (*mut i32, usize) {
    if v.is_empty() {
        return (ptr::null_mut(), 0);
    }
    let boxed = v.into_boxed_slice();
    let len = boxed.len();
    (Box::into_raw(boxed) as *mut i32, len)
}

fn services_response_to_result(resp: ServicesResponse) -> dd_discovery_result {
    let (injected_pids, injected_pids_len) = vec_i32_to_raw(resp.injected_pids);
    let (gpu_pids, gpu_pids_len) = vec_i32_to_raw(resp.gpu_pids);

    let services_vec: Vec<dd_service> = resp.services.into_iter().map(dd_service::from).collect();
    let (services, services_len) = if services_vec.is_empty() {
        (ptr::null_mut(), 0)
    } else {
        let boxed = services_vec.into_boxed_slice();
        let len = boxed.len();
        (Box::into_raw(boxed) as *mut dd_service, len)
    };

    dd_discovery_result {
        services,
        services_len,
        injected_pids,
        injected_pids_len,
        gpu_pids,
        gpu_pids_len,
    }
}

// ---------------------------------------------------------------------------
// Exported C ABI functions
// ---------------------------------------------------------------------------

/// Convert a C array of PIDs to `Option<Vec<i32>>`.
///
/// # Safety
/// If `ptr` is non-NULL, it must point to a valid array of `len` i32 values.
unsafe fn pids_from_c(ptr: *const i32, len: usize) -> Option<Vec<i32>> {
    if ptr.is_null() || len == 0 {
        return None;
    }
    // SAFETY: Caller guarantees ptr points to len valid i32 values.
    let slice = unsafe { std::slice::from_raw_parts(ptr, len) };
    Some(slice.to_vec())
}

/// Run service discovery and return a heap-allocated result.
///
/// # Parameters
/// - `new_pids` / `new_pids_len`: optional slice of PIDs to fully scan.
///   Pass NULL + 0 for none.
/// - `heartbeat_pids` / `heartbeat_pids_len`: optional slice of PIDs for heartbeat.
///   Pass NULL + 0 for none.
///
/// # Returns
/// Pointer to a `dd_discovery_result`. The caller MUST pass it to
/// `dd_discovery_free` exactly once after reading the fields.
///
/// # Safety
/// - If `new_pids` is non-NULL, it must point to a valid array of `new_pids_len` i32 values.
/// - If `heartbeat_pids` is non-NULL, it must point to a valid array of `heartbeat_pids_len` i32 values.
/// - The returned pointer must be freed with `dd_discovery_free` exactly once.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dd_discovery_get_services(
    new_pids: *const i32,
    new_pids_len: usize,
    heartbeat_pids: *const i32,
    heartbeat_pids_len: usize,
) -> *mut dd_discovery_result {
    // SAFETY: caller guarantees new_pids points to a valid array.
    let new = unsafe { pids_from_c(new_pids, new_pids_len) };
    // SAFETY: caller guarantees heartbeat_pids points to a valid array.
    let heartbeat = unsafe { pids_from_c(heartbeat_pids, heartbeat_pids_len) };

    let params = Params {
        new_pids: new,
        heartbeat_pids: heartbeat,
    };

    let resp = services::get_services(params);
    let result = services_response_to_result(resp);

    Box::into_raw(Box::new(result))
}

/// Free a `dd_discovery_result` previously returned by `dd_discovery_get_services`.
///
/// # Safety
/// `result` must be a pointer returned by `dd_discovery_get_services` and must
/// not have been freed before. Passing NULL is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dd_discovery_free(result: *mut dd_discovery_result) {
    if result.is_null() {
        return;
    }

    // SAFETY: `result` was created by `Box::into_raw(Box::new(...))` in
    // `dd_discovery_get_services` and has not been freed yet.
    let result = unsafe { Box::from_raw(result) };

    // Free the services array and all nested allocations
    if !result.services.is_null() {
        // SAFETY: `result.services` came from `Box::into_raw` in `services_response_to_result`.
        let services = unsafe {
            Box::from_raw(ptr::slice_from_raw_parts_mut(
                result.services,
                result.services_len,
            ))
        };

        for service in services.iter() {
            // SAFETY: All service fields are either NULL or heap-allocated via `Box::into_raw`.
            unsafe {
                free_dd_service(service);
            }
        }
    }

    // Free the injected_pids array
    if !result.injected_pids.is_null() {
        // SAFETY: `result.injected_pids` came from `Box::into_raw` in `services_response_to_result`.
        let _injected = unsafe {
            Box::from_raw(ptr::slice_from_raw_parts_mut(
                result.injected_pids,
                result.injected_pids_len,
            ))
        };
    }

    if !result.gpu_pids.is_null() {
        // SAFETY: `result.gpu_pids` came from `Box::into_raw` in `services_response_to_result`.
        let _gpu = unsafe {
            Box::from_raw(ptr::slice_from_raw_parts_mut(
                result.gpu_pids,
                result.gpu_pids_len,
            ))
        };
    }
}

/// Free all heap allocations within a `dd_service`.
///
/// # Safety
/// All pointer fields must be either NULL or valid heap pointers from `Box::into_raw`.
unsafe fn free_dd_service(service: &dd_service) {
    let dd_service {
        pid: _,
        generated_name,
        generated_name_source,
        additional_generated_names,
        tracer_metadata,
        ust,
        tcp_ports,
        udp_ports,
        log_files,
        apm_instrumentation: _,
        language,
        service_type,
        has_nvidia_gpu: _,
    } = service;
    // SAFETY: Caller guarantees pointers are from `Box::into_raw` or NULL.
    unsafe {
        free_dd_str(generated_name);
        free_dd_str(generated_name_source);
        free_dd_strs(additional_generated_names);
        free_dd_tracer_metadata_slice(tracer_metadata);
        free_dd_ust(ust);
        free_dd_u16_slice(tcp_ports);
        free_dd_u16_slice(udp_ports);
        free_dd_strs(log_files);
        free_dd_str(language);
        free_dd_str(service_type);
    }
}

/// Free a `dd_str` if it points to heap-allocated data.
///
/// # Safety
/// `s.data` must be either NULL or a valid pointer from `Box::into_raw(Box<[u8]>)`.
unsafe fn free_dd_str(s: &dd_str) {
    if !s.data.is_null() {
        // SAFETY: Caller guarantees this came from `Box::into_raw(boxed_slice)`.
        let _boxed =
            unsafe { Box::from_raw(ptr::slice_from_raw_parts_mut(s.data as *mut u8, s.len)) };
    }
}

/// Free a `dd_strs` array and all contained strings.
///
/// # Safety
/// `s.data` must be either NULL or a valid pointer from `Box::into_raw(Box<[dd_str]>)`,
/// and each contained `dd_str` must have valid heap pointers.
unsafe fn free_dd_strs(s: &dd_strs) {
    if !s.data.is_null() {
        // SAFETY: Caller guarantees this came from `Box::into_raw`.
        let strs =
            unsafe { Box::from_raw(ptr::slice_from_raw_parts_mut(s.data as *mut dd_str, s.len)) };

        for s in strs.iter() {
            // SAFETY: Each element was created via `dd_str::from` which uses `Box::into_raw`.
            unsafe {
                free_dd_str(s);
            }
        }
    }
}

/// Free a `dd_u16_slice`.
///
/// # Safety
/// `s.data` must be either NULL or a valid pointer from `Box::into_raw(Box<[u16]>)`.
unsafe fn free_dd_u16_slice(s: &dd_u16_slice) {
    if !s.data.is_null() {
        // SAFETY: Caller guarantees this came from `Box::into_raw`.
        let _boxed =
            unsafe { Box::from_raw(ptr::slice_from_raw_parts_mut(s.data as *mut u16, s.len)) };
    }
}

/// Free a `dd_ust` struct (3 `dd_str` fields).
///
/// # Safety
/// All `dd_str` fields must be valid per `free_dd_str` requirements.
unsafe fn free_dd_ust(ust: &dd_ust) {
    let dd_ust {
        service,
        env,
        version,
    } = ust;
    // SAFETY: Caller guarantees valid heap pointers or NULL.
    unsafe {
        free_dd_str(service);
        free_dd_str(env);
        free_dd_str(version);
    }
}

/// Free a `dd_tracer_metadata_slice` array and all nested data.
///
/// # Safety
/// `s.data` must be either NULL or a valid pointer from `Box::into_raw(Box<[dd_tracer_metadata]>)`,
/// and each metadata struct's fields must be valid.
unsafe fn free_dd_tracer_metadata_slice(s: &dd_tracer_metadata_slice) {
    if !s.data.is_null() {
        // SAFETY: Caller guarantees this came from `Box::into_raw`.
        let metadata_slice = unsafe {
            Box::from_raw(ptr::slice_from_raw_parts_mut(
                s.data as *mut dd_tracer_metadata,
                s.len,
            ))
        };

        for metadata in metadata_slice.iter() {
            // SAFETY: All fields are valid `dd_str` pointers.
            unsafe {
                free_dd_tracer_metadata(metadata);
            }
        }
    }
}

/// Free all `dd_str` fields in a `dd_tracer_metadata`.
///
/// # Safety
/// All `dd_str` fields must be valid per `free_dd_str` requirements.
unsafe fn free_dd_tracer_metadata(metadata: &dd_tracer_metadata) {
    let dd_tracer_metadata {
        schema_version: _,
        runtime_id,
        tracer_language,
        tracer_version,
        hostname,
        service_name,
        service_env,
        service_version,
    } = metadata;
    // SAFETY: Caller guarantees valid heap pointers or NULL.
    unsafe {
        free_dd_str(runtime_id);
        free_dd_str(tracer_language);
        free_dd_str(tracer_version);
        free_dd_str(hostname);
        free_dd_str(service_name);
        free_dd_str(service_env);
        free_dd_str(service_version);
    }
}

#[cfg(test)]
#[allow(
    clippy::unwrap_used,
    clippy::undocumented_unsafe_blocks,
    clippy::bool_assert_comparison,
    clippy::indexing_slicing
)]
mod tests {
    use super::*;
    use crate::service_name::ServiceNameSource;
    use crate::{Language, Service, ServicesResponse, TracerMetadata, UST};

    /// Helper to convert a `dd_str` to a Rust `&str` for verification.
    unsafe fn dd_str_to_str(s: &dd_str) -> &str {
        if s.data.is_null() {
            return "";
        }
        let slice = unsafe { std::slice::from_raw_parts(s.data.cast::<u8>(), s.len) };
        std::str::from_utf8(slice).unwrap()
    }

    #[test]
    fn empty_response() {
        let resp = ServicesResponse {
            services: vec![],
            injected_pids: vec![],
            gpu_pids: vec![],
        };
        let result = services_response_to_result(resp);

        assert!(result.services.is_null());
        assert_eq!(result.services_len, 0);
        assert!(result.injected_pids.is_null());
        assert_eq!(result.injected_pids_len, 0);
        assert!(result.gpu_pids.is_null());
        assert_eq!(result.gpu_pids_len, 0);

        // Verify free does not crash
        let ptr = Box::into_raw(Box::new(result));
        unsafe { dd_discovery_free(ptr) };
    }

    #[test]
    fn populated_response() {
        let resp = ServicesResponse {
            services: vec![Service {
                pid: 1234,
                generated_name: Some("test-service".to_string()),
                generated_name_source: Some(ServiceNameSource::CommandLine),
                additional_generated_names: vec!["alt1".to_string(), "alt2".to_string()],
                tracer_metadata: vec![TracerMetadata {
                    schema_version: 1,
                    runtime_id: Some("runtime123".to_string()),
                    tracer_language: Language::Python,
                    tracer_version: "1.0.0".to_string(),
                    hostname: "localhost".to_string(),
                    service_name: Some("my-service".to_string()),
                    service_env: Some("prod".to_string()),
                    service_version: Some("2.0.0".to_string()),
                }],
                ust: UST {
                    service: Some("ust-service".to_string()),
                    env: Some("staging".to_string()),
                    version: Some("3.0.0".to_string()),
                },
                tcp_ports: Some(vec![8080, 8443]),
                udp_ports: Some(vec![9000]),
                log_files: vec!["/var/log/app.log".to_string()],
                apm_instrumentation: true,
                language: Language::Python,
                service_type: "web_service".to_string(),
                has_nvidia_gpu: true,
            }],
            injected_pids: vec![5678, 9012],
            gpu_pids: vec![1111, 2222],
        };

        let result = services_response_to_result(resp);

        // Verify services
        assert!(!result.services.is_null());
        assert_eq!(result.services_len, 1);

        // SAFETY: We just created this with one service
        let service = unsafe { &*result.services };
        assert_eq!(service.pid, 1234);
        assert_eq!(
            unsafe { dd_str_to_str(&service.generated_name) },
            "test-service"
        );
        assert_eq!(
            unsafe { dd_str_to_str(&service.generated_name_source) },
            "command-line"
        );
        assert_eq!(service.apm_instrumentation, true);
        assert_eq!(unsafe { dd_str_to_str(&service.language) }, "python");
        assert_eq!(
            unsafe { dd_str_to_str(&service.service_type) },
            "web_service"
        );
        assert_eq!(service.has_nvidia_gpu, true);

        // Verify additional_generated_names
        assert!(!service.additional_generated_names.data.is_null());
        assert_eq!(service.additional_generated_names.len, 2);
        let names = unsafe {
            std::slice::from_raw_parts(
                service.additional_generated_names.data,
                service.additional_generated_names.len,
            )
        };
        assert_eq!(unsafe { dd_str_to_str(&names[0]) }, "alt1");
        assert_eq!(unsafe { dd_str_to_str(&names[1]) }, "alt2");

        // Verify tracer_metadata
        assert!(!service.tracer_metadata.data.is_null());
        assert_eq!(service.tracer_metadata.len, 1);
        let metadata = unsafe { &*service.tracer_metadata.data };
        assert_eq!(metadata.schema_version, 1);
        assert_eq!(unsafe { dd_str_to_str(&metadata.runtime_id) }, "runtime123");
        assert_eq!(
            unsafe { dd_str_to_str(&metadata.tracer_language) },
            "python"
        );

        // Verify UST
        assert_eq!(
            unsafe { dd_str_to_str(&service.ust.service) },
            "ust-service"
        );
        assert_eq!(unsafe { dd_str_to_str(&service.ust.env) }, "staging");
        assert_eq!(unsafe { dd_str_to_str(&service.ust.version) }, "3.0.0");

        // Verify ports
        assert!(!service.tcp_ports.data.is_null());
        assert_eq!(service.tcp_ports.len, 2);
        let tcp =
            unsafe { std::slice::from_raw_parts(service.tcp_ports.data, service.tcp_ports.len) };
        assert_eq!(tcp, &[8080, 8443]);

        assert!(!service.udp_ports.data.is_null());
        assert_eq!(service.udp_ports.len, 1);
        let udp =
            unsafe { std::slice::from_raw_parts(service.udp_ports.data, service.udp_ports.len) };
        assert_eq!(udp, &[9000]);

        // Verify log_files
        assert!(!service.log_files.data.is_null());
        assert_eq!(service.log_files.len, 1);
        let logs =
            unsafe { std::slice::from_raw_parts(service.log_files.data, service.log_files.len) };
        assert_eq!(unsafe { dd_str_to_str(&logs[0]) }, "/var/log/app.log");

        // Verify injected_pids
        assert!(!result.injected_pids.is_null());
        assert_eq!(result.injected_pids_len, 2);
        let pids =
            unsafe { std::slice::from_raw_parts(result.injected_pids, result.injected_pids_len) };
        assert_eq!(pids, &[5678, 9012]);

        // Verify gpu_pids
        assert!(!result.gpu_pids.is_null());
        assert_eq!(result.gpu_pids_len, 2);
        let gpu_pids = unsafe { std::slice::from_raw_parts(result.gpu_pids, result.gpu_pids_len) };
        assert_eq!(gpu_pids, &[1111, 2222]);

        // Free and verify no crash
        let ptr = Box::into_raw(Box::new(result));
        unsafe { dd_discovery_free(ptr) };
    }

    #[test]
    fn empty_optionals() {
        let resp = ServicesResponse {
            services: vec![Service {
                pid: 42,
                generated_name: Some("minimal".to_string()),
                generated_name_source: None,
                additional_generated_names: vec![],
                tracer_metadata: vec![],
                ust: UST {
                    service: None,
                    env: None,
                    version: None,
                },
                tcp_ports: None,
                udp_ports: Some(vec![]),
                log_files: vec![],
                apm_instrumentation: false,
                language: Language::Unknown,
                service_type: "unknown".to_string(),
                has_nvidia_gpu: false,
            }],
            injected_pids: vec![],
            gpu_pids: vec![],
        };

        let result = services_response_to_result(resp);

        assert!(!result.services.is_null());
        assert_eq!(result.services_len, 1);

        let service = unsafe { &*result.services };
        assert_eq!(service.pid, 42);
        assert_eq!(unsafe { dd_str_to_str(&service.generated_name) }, "minimal");

        // Verify empty/None fields are NULL
        assert!(service.generated_name_source.data.is_null());
        assert!(service.additional_generated_names.data.is_null());
        assert_eq!(service.additional_generated_names.len, 0);
        assert!(service.tracer_metadata.data.is_null());
        assert_eq!(service.tracer_metadata.len, 0);
        assert!(service.ust.service.data.is_null());
        assert!(service.ust.env.data.is_null());
        assert!(service.ust.version.data.is_null());
        assert!(service.tcp_ports.data.is_null());
        assert_eq!(service.tcp_ports.len, 0);
        assert!(service.udp_ports.data.is_null());
        assert_eq!(service.udp_ports.len, 0);
        assert!(service.log_files.data.is_null());
        assert_eq!(service.log_files.len, 0);
        assert_eq!(service.apm_instrumentation, false);
        assert_eq!(service.has_nvidia_gpu, false);

        assert!(result.injected_pids.is_null());
        assert_eq!(result.injected_pids_len, 0);
        assert!(result.gpu_pids.is_null());
        assert_eq!(result.gpu_pids_len, 0);

        let ptr = Box::into_raw(Box::new(result));
        unsafe { dd_discovery_free(ptr) };
    }

    #[test]
    fn free_null_is_safe() {
        // Verify passing NULL to free is a no-op
        unsafe { dd_discovery_free(std::ptr::null_mut()) };
    }

    #[test]
    fn pids_from_c_helper() {
        // Test NULL pointer
        let result = unsafe { pids_from_c(std::ptr::null(), 0) };
        assert_eq!(result, None);

        let result = unsafe { pids_from_c(std::ptr::null(), 10) };
        assert_eq!(result, None);

        // Test zero length
        let pids = [1, 2, 3];
        let result = unsafe { pids_from_c(pids.as_ptr(), 0) };
        assert_eq!(result, None);

        // Test valid conversion
        let pids: [i32; 3] = [100, 200, 300];
        let result = unsafe { pids_from_c(pids.as_ptr(), pids.len()) };
        assert_eq!(result, Some(vec![100, 200, 300]));
    }
}
