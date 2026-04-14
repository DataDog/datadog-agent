// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::path::PathBuf;

use serde::Serialize;

use crate::apm;
use crate::comm;
use crate::envs;
use crate::fs::SubDirFs;
use crate::language::Language;
use crate::params::Params;
use crate::ports::{self, ParsingContext};
use crate::procfs::maps::MapsInfo;
use crate::procfs::{self, Cmdline, Exe, fd::OpenFilesInfo};
use crate::service_name::ServiceNameSource;
use crate::tracer_metadata::TracerMetadata;
use crate::ust::UST;
use crate::{service_name, tracer_metadata};

#[derive(Debug, Serialize)]
pub struct ServicesResponse {
    pub services: Vec<Service>,
    pub injected_pids: Vec<i32>,
    pub gpu_pids: Vec<i32>,
}

impl ServicesResponse {
    fn new() -> Self {
        ServicesResponse {
            services: Vec::new(),
            injected_pids: Vec::new(),
            gpu_pids: Vec::new(),
        }
    }
}

#[derive(Debug, Default, Serialize)]
pub struct Service {
    pub pid: i32,
    pub generated_name: Option<String>,
    pub generated_name_source: Option<ServiceNameSource>,
    pub additional_generated_names: Vec<String>,
    pub tracer_metadata: Vec<TracerMetadata>,
    pub ust: UST,
    pub tcp_ports: Option<Vec<u16>>,
    pub udp_ports: Option<Vec<u16>>,

    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub log_files: Vec<String>,
    pub apm_instrumentation: bool,
    pub language: Option<Language>,
}

// getServices processes categorized PID lists and returns service information
// for each. This is used by the /services endpoint which accepts explicit PID
// lists. The caller (the Core-Agent process collector) will handle the retry.
pub fn get_services(params: Params) -> ServicesResponse {
    let mut resp = ServicesResponse::new();
    let mut context = ParsingContext::new();

    // Process new PIDs with full service info collection
    if let Some(new_pids) = &params.new_pids {
        for pid in new_pids {
            // Read /proc/pid/maps once and extract all information in one pass.
            // On failure, use defaults so the PID still tries port/service
            // detection; only maps-dependent information is potentially lost.
            let maps_info = procfs::maps::read_maps_info(*pid).unwrap_or_default();

            // Check for APM injector even if process is not detected as a service.
            if maps_info.has_apm_injector {
                resp.injected_pids.push(*pid);
            }

            if comm::should_ignore_comm(*pid) {
                continue;
            }

            let Ok(open_files_info) = procfs::fd::get_open_files_info(*pid) else {
                continue;
            };

            if open_files_info.has_gpu_device || maps_info.has_gpu_nvidia {
                resp.gpu_pids.push(*pid);
            }

            if let Some(service) = get_service(*pid, &mut context, &open_files_info, &maps_info) {
                resp.services.push(service);
            }
        }
    }

    if let Some(heartbeat_pids) = &params.heartbeat_pids {
        for pid in heartbeat_pids {
            if comm::should_ignore_comm(*pid) {
                continue;
            }

            if let Some(service) = get_heartbeat_service(*pid, &mut context) {
                resp.services.push(service);
            }
        }
    }

    resp
}

/// Reads tracer metadata from memfd paths and returns only the newest one
/// (by file modification time). When there is only one memfd, skips the stat
/// call.
fn get_newest_tracer_metadata(memfd_paths: &[PathBuf]) -> Option<TracerMetadata> {
    if memfd_paths.len() <= 1 {
        return memfd_paths
            .first()
            .and_then(|path| tracer_metadata::get_tracer_metadata_from_path(path).ok());
    }

    memfd_paths
        .iter()
        .filter_map(|path| {
            let mtime = std::fs::metadata(path).ok()?.modified().ok()?;
            let tm = tracer_metadata::get_tracer_metadata_from_path(path).ok()?;
            Some((tm, mtime))
        })
        .max_by(|(tm_a, t_a), (tm_b, t_b)| {
            (t_a, tm_a.runtime_id.as_deref()).cmp(&(t_b, tm_b.runtime_id.as_deref()))
        })
        .map(|(m, _)| m)
}

fn get_service(
    pid: i32,
    context: &mut ParsingContext,
    open_files_info: &OpenFilesInfo,
    maps_info: &MapsInfo,
) -> Option<Service> {
    let log_files = procfs::fd::get_log_files(pid, &open_files_info.logs);

    let (tcp_ports, udp_ports) = ports::get(context, pid, &open_files_info.sockets);

    let has_log_candidates = !open_files_info.logs.is_empty();

    if tcp_ports.is_none()
        && udp_ports.is_none()
        && open_files_info.tracer_memfds.is_empty()
        && !has_log_candidates
    {
        return None;
    }

    let cmdline = Cmdline::get(pid).ok()?;
    let exe = Exe::get(pid).ok()?;
    let tracer_metadata = get_newest_tracer_metadata(&open_files_info.tracer_memfds);
    let language = tracer_metadata
        .as_ref()
        .and_then(|m| Language::from_tracer_str(&m.tracer_language))
        .or_else(|| Language::detect(pid, &exe, &cmdline, open_files_info, maps_info));

    // Collect environment variables
    let envs = envs::get_target_envs(pid).ok()?;

    // Open filesystem for the process at /proc/<pid>/root
    let proc_root = procfs::root_path().join(pid.to_string()).join("root");
    let fs = SubDirFs::new(&proc_root).ok()?;

    // Create detection context for service name generation
    let mut ctx = service_name::DetectionContext::new(pid, envs.clone(), &fs);
    let name_metadata = service_name::get(language.as_ref(), &cmdline, &mut ctx);

    // Detect APM instrumentation
    // If tracer metadata exists, the service is definitely instrumented
    let apm_instrumentation =
        tracer_metadata.is_some() || apm::detect(language.as_ref(), &cmdline, &envs, maps_info);

    Some(Service {
        pid,
        generated_name: name_metadata.as_ref().map(|meta| meta.name.clone()),
        generated_name_source: name_metadata.as_ref().map(|meta| meta.source.clone()),
        additional_generated_names: name_metadata
            .map(|meta| meta.additional_names)
            .unwrap_or_default(),
        tracer_metadata: tracer_metadata.into_iter().collect(),
        ust: UST::from_envs(&envs),
        tcp_ports,
        udp_ports,
        log_files,
        apm_instrumentation,
        language,
    })
}

// GPU state is not re-detected on heartbeat; the caller preserves it from the initial detection.
fn get_heartbeat_service(pid: i32, context: &mut ParsingContext) -> Option<Service> {
    let open_files_info = procfs::fd::get_open_files_info(pid).ok()?;

    let log_files = procfs::fd::get_log_files(pid, &open_files_info.logs);

    let (tcp_ports, udp_ports) = ports::get(context, pid, &open_files_info.sockets);

    if tcp_ports.is_none()
        && udp_ports.is_none()
        && open_files_info.tracer_memfds.is_empty()
        && open_files_info.logs.is_empty()
    {
        return None;
    }

    Some(Service {
        pid,
        tcp_ports,
        udp_ports,
        log_files,
        ..Default::default()
    })
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;
    use crate::params::Params;

    #[cfg(target_os = "linux")]
    mod log_file_integration {
        use super::*;
        use std::fs::OpenOptions;
        use tempfile::TempDir;

        #[test]
        fn test_service_with_only_logs() {
            let temp_dir = TempDir::new().expect("Failed to create temp dir");
            let log_path = temp_dir.path().join("test-service-only-logs.log");

            let _log_file = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&log_path)
                .expect("Failed to open log file");

            std::fs::write(&log_path, "Service started\n").expect("Failed to write to log");

            let pid = std::process::id().cast_signed();

            let open_files_info =
                procfs::fd::get_open_files_info(pid).expect("Failed to collect open files");

            let has_log_candidate = open_files_info.logs.iter().any(|fd_path| {
                fd_path
                    .path
                    .to_str()
                    .is_some_and(|p| p.contains("test-service-only-logs.log"))
            });

            assert!(
                has_log_candidate,
                "Expected to find self-generated log candidate"
            );

            let _validated_logs = procfs::fd::get_log_files(pid, &open_files_info.logs);

            assert!(
                !open_files_info.logs.is_empty(),
                "Process with open log files should have log candidates"
            );
        }

        #[test]
        fn test_service_with_invalid_log_candidates() {
            let temp_dir = TempDir::new().expect("Failed to create temp dir");
            let log_path = temp_dir.path().join("test-invalid-logs.log");

            std::fs::write(&log_path, "test content").expect("Failed to create file");

            let _log_file = OpenOptions::new()
                .read(true)
                .open(&log_path)
                .expect("Failed to open log file");

            let pid = std::process::id().cast_signed();

            let proc_path = procfs::root_path().join(pid.to_string());
            assert!(
                proc_path.exists(),
                "Expected /proc entry for current pid during test"
            );

            let open_files_info =
                procfs::fd::get_open_files_info(pid).expect("Failed to collect open files");

            let candidates: Vec<_> = open_files_info
                .logs
                .iter()
                .filter(|fd_path| {
                    fd_path
                        .path
                        .to_str()
                        .is_some_and(|p| p.contains("test-invalid-logs.log"))
                })
                .collect();

            if !candidates.is_empty() {
                let validated_logs = procfs::fd::get_log_files(pid, &open_files_info.logs);
                let contains_invalid = validated_logs
                    .iter()
                    .any(|p| p.contains("test-invalid-logs.log"));

                assert!(
                    !contains_invalid,
                    "Read-only log files should be filtered out by flag validation"
                );
            }
        }

        #[test]
        fn test_log_deduplication_in_service() {
            let temp_dir = TempDir::new().expect("Failed to create temp dir");
            let log_path = temp_dir.path().join("test-dedup.log");

            let _log_file1 = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&log_path)
                .expect("Failed to open log file 1");

            let _log_file2 = OpenOptions::new()
                .append(true)
                .open(&log_path)
                .expect("Failed to open log file 2");

            let pid = std::process::id().cast_signed();

            let proc_path = procfs::root_path().join(pid.to_string());
            assert!(
                proc_path.exists(),
                "Expected /proc entry for current pid during test"
            );

            let open_files_info =
                procfs::fd::get_open_files_info(pid).expect("Failed to collect open files");

            let candidates: Vec<_> = open_files_info
                .logs
                .iter()
                .filter(|fd_path| {
                    fd_path
                        .path
                        .to_str()
                        .is_some_and(|p| p.contains("test-dedup.log"))
                })
                .collect();

            if !candidates.is_empty() {
                let validated_logs = procfs::fd::get_log_files(pid, &open_files_info.logs);

                let count = validated_logs
                    .iter()
                    .filter(|p| p.contains("test-dedup.log"))
                    .count();

                assert!(
                    count <= 1,
                    "Same log file should be deduplicated to single entry, found {} entries",
                    count
                );
            }
        }
    }

    mod log_files_field_validation {
        use super::*;

        #[test]
        fn test_log_files_serialization_skipped_when_empty() {
            let service = Service {
                pid: 123,
                log_files: vec![],
                ..Default::default()
            };

            let json = serde_json::to_string(&service).unwrap();
            assert!(
                !json.contains("log_files"),
                "log_files should not be serialized when empty"
            );
        }

        #[test]
        fn test_log_files_serialization_present_when_populated() {
            let service = Service {
                pid: 123,
                log_files: vec!["/var/log/app.log".to_string()],
                ..Default::default()
            };

            let json = serde_json::to_string(&service).unwrap();
            assert!(
                json.contains("log_files"),
                "log_files should be serialized when populated"
            );
            assert!(
                json.contains("/var/log/app.log"),
                "log file path should be in JSON"
            );
        }
    }

    #[test]
    fn test_unreadable_pid_not_in_gpu_pids() {
        let params = Params {
            new_pids: Some(vec![i32::MAX]),
            heartbeat_pids: None,
        };
        let resp = get_services(params);
        assert!(resp.gpu_pids.is_empty());
        assert!(resp.services.is_empty());
    }
}
