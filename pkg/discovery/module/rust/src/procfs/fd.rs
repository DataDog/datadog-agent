// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! The fd module contains helpers and types to represent information located
//! in /proc/<pid>/fd

use std::fs::{read_dir, read_link};
use std::path::{Path, PathBuf};

use log::trace;

use crate::procfs::root_path;

const O_WRONLY: u32 = 0o1;
const O_APPEND: u32 = 0o2000;

#[derive(Debug)]
pub struct OpenFilesInfo {
    pub sockets: Vec<u64>,
    pub logs: Vec<FdPath>,
    pub tracer_memfd: Option<PathBuf>,
    pub memfd_path: Option<PathBuf>,
    pub has_gpu_device: bool,
}

#[derive(Debug)]
pub struct FdPath {
    pub fd: String,
    pub path: PathBuf,
}

#[derive(Debug, Clone)]
struct FdInfo {
    flags: u32,
}

pub fn get_open_files_info(pid: i32) -> Result<OpenFilesInfo, std::io::Error> {
    let fd_path = root_path().join(pid.to_string()).join("fd");
    let mut result = OpenFilesInfo {
        sockets: Vec::new(),
        logs: Vec::new(),
        tracer_memfd: None,
        memfd_path: None,
        has_gpu_device: false,
    };

    read_dir(fd_path)?
        .filter_map(|entry_result| entry_result.ok())
        .filter_map(|entry| {
            let path = entry.path();
            let link = read_link(&path).ok()?;
            Some((path, link))
        })
        .for_each(|(entry, link)| {
            // Check for GPU device first (before link is moved)
            if !result.has_gpu_device && is_gpu_device(link.as_path()) {
                result.has_gpu_device = true;
                return;
            }

            if let Some(socket) = is_socket(link.as_path()) {
                result.sockets.push(socket);
            } else if is_logfile(link.as_path()) {
                if let Some(fd_path) = entry.to_str().map(|s| s.to_string()) {
                    result.logs.push(FdPath {
                        fd: fd_path,
                        path: link,
                    });
                }
            } else if is_tracer_memfd(link.as_path()) {
                result.tracer_memfd = Some(entry);
            } else if is_language_memfd(link.as_path()) {
                result.memfd_path = Some(entry);
            }
        });

    Ok(result)
}

pub fn get_log_files(pid: i32, candidates: &[FdPath]) -> Vec<String> {
    use std::collections::HashSet;

    let mut seen = HashSet::new();
    let mut logs = Vec::new();

    for candidate in candidates {
        if seen.contains(&candidate.path) {
            continue;
        }

        let Some(fd_num) = candidate.fd.rsplit('/').next() else {
            continue;
        };

        let Some(fd_info) = read_fdinfo(pid, fd_num) else {
            continue;
        };

        if !is_write_append_mode(fd_info.flags) {
            trace!(
                "Discarding log candidate {} for pid {} due to invalid flags: {:#o}",
                candidate.path.display(),
                pid,
                fd_info.flags
            );
            continue;
        }

        if !seen.insert(&candidate.path) {
            continue;
        }

        let path = candidate.path.to_string_lossy().into_owned();
        logs.push(path);
    }

    logs
}

fn is_write_append_mode(flags: u32) -> bool {
    let has_wronly = (flags & O_WRONLY) != 0;
    let has_append = (flags & O_APPEND) != 0;

    has_wronly && has_append
}

fn parse_fdinfo(content: &str) -> Option<FdInfo> {
    for line in content.lines() {
        let mut parts = line.split_whitespace();
        if parts.next()? == "flags:" {
            let flags_str = parts.next()?;
            let flags = u32::from_str_radix(flags_str, 8).ok()?;
            return Some(FdInfo { flags });
        }
    }

    None
}

fn read_fdinfo(pid: i32, fd_num: &str) -> Option<FdInfo> {
    let fdinfo_path = root_path()
        .join(pid.to_string())
        .join("fdinfo")
        .join(fd_num);

    let content = std::fs::read_to_string(fdinfo_path).ok()?;
    parse_fdinfo(&content)
}

fn is_socket(link: &Path) -> Option<u64> {
    const SOCKET_PREFIX: &str = "socket:[";

    let link_str = link.to_str()?;
    let link_str = link_str.strip_prefix(SOCKET_PREFIX)?;

    link_str.get(..link_str.len() - 1)?.parse().ok()
}

fn is_logfile(link: &Path) -> bool {
    // Files in /var/log are log files even if they don't end with .log.
    if link.starts_with("/var/log/") {
        // Ignore Kubernetes pods logs since they are collected by other means.
        return !link.starts_with("/var/log/pods");
    }

    // Check if file has .log extension OR is named exactly ".log" (matches Go implementation)
    // We don't use extension() here because it returns None when the file name is
    // exactly ".log", and the Go implementation treats ".log" as a valid log file.
    let mut has_log_extension = false;
    if let Some(file_name) = link.file_name().and_then(|name| name.to_str())
        && file_name.ends_with(".log")
    {
        has_log_extension = true;
    }
    // Ignore Docker container logs since they are collected by other means.
    has_log_extension && !link.starts_with("/var/lib/docker/containers")
}

fn is_tracer_memfd(link: &Path) -> bool {
    const TRACER_MEMFD_PREFIX: &str = "/memfd:datadog-tracer-info-";

    let Some(link_str) = link.to_str() else {
        return false;
    };

    link_str.starts_with(TRACER_MEMFD_PREFIX)
}

fn is_language_memfd(link: &Path) -> bool {
    const LANGUAGE_MEMFD_NAME: &str = "/memfd:dd_language_detected";

    let Some(link_str) = link.to_str() else {
        return false;
    };

    link_str
        .strip_prefix(LANGUAGE_MEMFD_NAME)
        .map(|rest| rest.is_empty() || rest.starts_with(' '))
        .unwrap_or(false)
}

fn is_gpu_device(link: &Path) -> bool {
    const NVIDIA_DEV_PREFIX: &[u8] = b"/dev/nvidia";

    let bytes = link.as_os_str().as_encoded_bytes();

    let Some(suffix) = bytes.strip_prefix(NVIDIA_DEV_PREFIX) else {
        return false;
    };

    suffix == b"ctl"
        || suffix == b"-uvm"
        || suffix == b"-uvm-tools"
        || suffix == b"-modeset"
        || suffix.starts_with(b"-caps/")
        || (!suffix.is_empty() && suffix.iter().all(|c| c.is_ascii_digit()))
}

#[cfg(test)]
mod tests {
    mod is_logfile {
        use std::path::Path;

        use crate::procfs::fd::is_logfile;

        #[test]
        fn valid_log_file() {
            assert!(is_logfile(Path::new("/tmp/foo/application.log")))
        }

        #[test]
        fn docker_container_log_excluded() {
            assert!(!is_logfile(Path::new(
                "/var/lib/docker/containers/abc123/abc123-json.log"
            )))
        }

        #[test]
        fn kubernetes_pod_log_excluded() {
            assert!(!is_logfile(Path::new(
                "/var/log/pods/namespace_pod_uid/container/0.log"
            )))
        }

        #[test]
        fn file_without_log_ext() {
            assert!(!is_logfile(Path::new("/usr/local/application.txt")))
        }

        #[test]
        fn file_with_ext_after_log() {
            assert!(!is_logfile(Path::new("/bar/tmp/application.log.gz")))
        }

        #[test]
        fn file_with_log_in_name_but_no_ext() {
            assert!(!is_logfile(Path::new("/foo/logfile.txt")))
        }

        #[test]
        fn var_log_with_log_ext() {
            assert!(is_logfile(Path::new("/var/log/messages")))
        }

        #[test]
        fn empty_path() {
            assert!(!is_logfile(Path::new("")))
        }

        #[test]
        fn path_with_only_log() {
            // .log has .log extension, so it IS a valid log file (matches Go implementation)
            assert!(is_logfile(Path::new(".log")))
        }

        #[test]
        fn var_logs_without_extension_should_not_match() {
            // /var/logs/messages (no .log extension, wrong dir) should NOT match
            assert!(!is_logfile(Path::new("/var/logs/messages")))
        }

        #[test]
        fn var_logfiles_without_extension_should_not_match() {
            // /var/logfiles/messages (no .log extension, wrong dir) should NOT match
            assert!(!is_logfile(Path::new("/var/logfiles/messages")))
        }

        #[test]
        fn var_log_without_extension_should_match() {
            // /var/log/messages (no .log extension, but in /var/log/) SHOULD match
            assert!(is_logfile(Path::new("/var/log/messages")))
        }

        #[test]
        fn var_logs_with_log_extension_should_match() {
            // Files with .log extension anywhere (except excluded dirs) should match
            assert!(is_logfile(Path::new("/var/logs/app.log")))
        }
    }

    mod is_language_memfd {
        use std::path::Path;

        use crate::procfs::fd::is_language_memfd;

        #[test]
        fn exact_match() {
            assert!(is_language_memfd(Path::new("/memfd:dd_language_detected")))
        }

        #[test]
        fn with_deleted_suffix() {
            assert!(is_language_memfd(Path::new(
                "/memfd:dd_language_detected (deleted)"
            )))
        }

        #[test]
        fn with_space_prefix() {
            assert!(is_language_memfd(Path::new("/memfd:dd_language_detected ")))
        }

        #[test]
        fn wrong_name() {
            assert!(!is_language_memfd(Path::new(
                "/memfd:dd_language_detected_wrong"
            )))
        }

        #[test]
        fn different_memfd() {
            assert!(!is_language_memfd(Path::new(
                "/memfd:datadog-tracer-info-123"
            )))
        }

        #[test]
        fn not_memfd() {
            assert!(!is_language_memfd(Path::new("/tmp/regular_file")))
        }

        #[test]
        fn empty_path() {
            assert!(!is_language_memfd(Path::new("")))
        }

        #[test]
        fn partial_match_should_not_detect() {
            assert!(!is_language_memfd(Path::new(
                "/memfd:dd_language_detected_v2"
            )))
        }

        #[test]
        fn no_space_after_prefix_should_not_match() {
            assert!(!is_language_memfd(Path::new(
                "/memfd:dd_language_detectedX"
            )))
        }

        #[test]
        fn double_space_should_match() {
            assert!(is_language_memfd(Path::new(
                "/memfd:dd_language_detected  (double space)"
            )))
        }
    }

    mod flag_validation {
        use super::super::is_write_append_mode;

        #[test]
        fn test_is_write_append_mode_wronly_append() {
            let flags = 0o2001;
            assert!(is_write_append_mode(flags));
        }

        #[test]
        fn test_is_write_append_mode_rdwr_append() {
            let flags = 0o2002;
            assert!(!is_write_append_mode(flags));
        }

        #[test]
        fn test_is_write_append_mode_wronly_no_append() {
            let flags = 0o1;
            assert!(!is_write_append_mode(flags));
        }

        #[test]
        fn test_is_write_append_mode_rdonly_append() {
            let flags = 0o2000;
            assert!(!is_write_append_mode(flags));
        }

        #[test]
        fn test_is_write_append_mode_rdonly() {
            let flags = 0o0;
            assert!(!is_write_append_mode(flags));
        }

        #[test]
        fn test_is_write_append_mode_with_other_flags() {
            let flags = 0o2002101;
            assert!(is_write_append_mode(flags));
        }
    }

    mod fdinfo_parsing {
        use super::super::parse_fdinfo;

        #[test]
        fn test_parse_fdinfo_complete() {
            let content = "pos:\t0\nflags:\t0100001\nmnt_id:\t25\n";
            let info = parse_fdinfo(content);
            assert!(info.is_some());
            if let Some(info) = info {
                assert_eq!(info.flags, 0o100001);
            }
        }

        #[test]
        fn test_parse_fdinfo_flags_only() {
            let content = "flags:\t02001\n";
            let info = parse_fdinfo(content);
            assert!(info.is_some());
            if let Some(info) = info {
                assert_eq!(info.flags, 0o2001);
            }
        }

        #[test]
        fn test_parse_fdinfo_invalid_format() {
            let content = "invalid content";
            assert!(parse_fdinfo(content).is_none());
        }

        #[test]
        fn test_parse_fdinfo_missing_flags() {
            let content = "pos:\t0\nmnt_id:\t25\n";
            assert!(parse_fdinfo(content).is_none());
        }

        #[test]
        fn test_parse_fdinfo_invalid_octal() {
            let content = "flags:\t12345xyz\n";
            assert!(parse_fdinfo(content).is_none());
        }
    }

    mod get_log_files {
        use std::path::PathBuf;

        use super::super::{FdPath, get_log_files};

        #[cfg(target_os = "linux")]
        use {
            crate::procfs::fd::get_open_files_info,
            std::fs::{File, OpenOptions},
            std::io::Write,
        };

        #[test]
        fn returns_empty_for_empty_candidates() {
            let candidates: Vec<FdPath> = vec![];
            let result = get_log_files(1, &candidates);
            assert!(result.is_empty());
        }

        #[test]
        fn returns_empty_when_fdinfo_not_accessible() {
            let nonexistent_pid = i32::MAX;
            let candidates = vec![
                FdPath {
                    fd: format!("/proc/{}/fd/3", nonexistent_pid),
                    path: PathBuf::from("/var/log/app.log"),
                },
                FdPath {
                    fd: format!("/proc/{}/fd/5", nonexistent_pid),
                    path: PathBuf::from("/var/log/app.log"),
                },
            ];

            let result = get_log_files(nonexistent_pid, &candidates);
            assert!(result.is_empty());
        }

        #[test]
        fn handles_invalid_fd_path() {
            let candidates = vec![FdPath {
                fd: "invalid".to_string(),
                path: PathBuf::from("/var/log/app.log"),
            }];

            let result = get_log_files(1234, &candidates);
            assert!(result.is_empty());
        }

        #[test]
        #[cfg(target_os = "linux")]
        #[allow(clippy::expect_used)]
        fn test_get_log_files_integration() {
            use tempfile::TempDir;

            let temp_dir = TempDir::new().expect("Failed to create temp dir");
            let temp_path = temp_dir.path();

            let valid_log = temp_path.join("test.log");
            let _file1 = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&valid_log)
                .expect("Failed to open valid log");

            let no_append_log = temp_path.join("noappend.log");
            let _file2 = OpenOptions::new()
                .create(true)
                .truncate(true)
                .write(true)
                .open(&no_append_log)
                .expect("Failed to open no-append log");

            let readwrite_log = temp_path.join("readwrite.log");
            let _file3 = OpenOptions::new()
                .create(true)
                .truncate(true)
                .read(true)
                .write(true)
                .open(&readwrite_log)
                .expect("Failed to open readwrite log");

            let wrong_ext = temp_path.join("test.log.txt");
            let _file4 = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&wrong_ext)
                .expect("Failed to open wrong-ext log");

            let readonly_log = temp_path.join("read.log");
            {
                let mut file =
                    File::create(&readonly_log).expect("Failed to create readonly log file");
                file.write_all(b"test")
                    .expect("Failed to seed readonly log file");
            }
            let _file5 = OpenOptions::new()
                .read(true)
                .open(&readonly_log)
                .expect("Failed to reopen readonly log");

            let long_name = "a".repeat(128) + ".log";
            let long_log = temp_path.join(&long_name);
            let _file6 = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&long_log)
                .expect("Failed to open long log");

            let _file7 = OpenOptions::new()
                .append(true)
                .open(&valid_log)
                .expect("Failed to reopen valid log");

            let pid = std::process::id().cast_signed();

            let open_files_info = get_open_files_info(pid).expect("Failed to collect open files");

            let temp_path_str = temp_path
                .to_str()
                .expect("Temp path should be valid UTF-8 for test");

            let our_logs: Vec<FdPath> = open_files_info
                .logs
                .into_iter()
                .filter(|fd_path| {
                    fd_path
                        .path
                        .to_str()
                        .map(|s| s.starts_with(temp_path_str))
                        .unwrap_or(false)
                })
                .collect();

            let result = get_log_files(pid, &our_logs);
            assert!(!result.is_empty());
            assert!(result.len() <= 2);

            let valid_log_str = valid_log
                .to_str()
                .expect("valid_log path should be valid UTF-8");
            assert!(result.iter().any(|p| p == valid_log_str));
            let no_append_str = no_append_log
                .to_str()
                .expect("no_append_log path should be valid UTF-8");
            let readwrite_str = readwrite_log
                .to_str()
                .expect("readwrite_log path should be valid UTF-8");
            let wrong_ext_str = wrong_ext
                .to_str()
                .expect("wrong_ext path should be valid UTF-8");
            let readonly_str = readonly_log
                .to_str()
                .expect("readonly_log path should be valid UTF-8");

            assert!(!result.iter().any(|p| p == no_append_str));
            assert!(!result.iter().any(|p| p == readwrite_str));
            assert!(!result.iter().any(|p| p == wrong_ext_str));
            assert!(!result.iter().any(|p| p == readonly_str));

            let count = result.iter().filter(|p| *p == valid_log_str).count();
            assert_eq!(count, 1);
        }
    }

    mod is_gpu_device {
        use std::path::Path;

        use crate::procfs::fd::is_gpu_device;

        #[test]
        fn test_nvidia_numbered_devices() {
            // NVIDIA GPU devices with numbers
            assert!(is_gpu_device(Path::new("/dev/nvidia0")));
            assert!(is_gpu_device(Path::new("/dev/nvidia1")));
            assert!(is_gpu_device(Path::new("/dev/nvidia2")));
            assert!(is_gpu_device(Path::new("/dev/nvidia10")));
            assert!(is_gpu_device(Path::new("/dev/nvidia123")));
        }

        #[test]
        fn test_nvidia_special_devices() {
            // NVIDIA special devices
            assert!(is_gpu_device(Path::new("/dev/nvidiactl")));
            assert!(is_gpu_device(Path::new("/dev/nvidia-uvm")));
            assert!(is_gpu_device(Path::new("/dev/nvidia-uvm-tools")));
            assert!(is_gpu_device(Path::new("/dev/nvidia-modeset")));
        }

        #[test]
        fn test_nvidia_mig_devices() {
            // NVIDIA MIG (Multi-Instance GPU) devices
            assert!(is_gpu_device(Path::new("/dev/nvidia-caps/nvidia-cap1")));
            assert!(is_gpu_device(Path::new("/dev/nvidia-caps/nvidia-cap2")));
            assert!(is_gpu_device(Path::new("/dev/nvidia-caps/nvidia-cap10")));
        }

        #[test]
        fn test_non_gpu_devices() {
            // Standard system devices
            assert!(!is_gpu_device(Path::new("/dev/null")));
            assert!(!is_gpu_device(Path::new("/dev/zero")));
            assert!(!is_gpu_device(Path::new("/dev/pts/0")));
            assert!(!is_gpu_device(Path::new("/dev/tty")));
            assert!(!is_gpu_device(Path::new("socket:[12345]")));
            assert!(!is_gpu_device(Path::new("pipe:[12345]")));
            assert!(!is_gpu_device(Path::new("anon_inode:[eventfd]")));
        }

        #[test]
        fn test_invalid_nvidia_names() {
            // Invalid NVIDIA device names
            assert!(!is_gpu_device(Path::new("/dev/nvidia")));
            assert!(!is_gpu_device(Path::new("/dev/nvidia-foo")));
            assert!(!is_gpu_device(Path::new("/dev/nvidiaXYZ")));
            assert!(!is_gpu_device(Path::new("/dev/nvidia0abc")));
            assert!(!is_gpu_device(Path::new("/dev/nvidia-")));
            assert!(!is_gpu_device(Path::new("/dev/nvidiactl2")));
            assert!(!is_gpu_device(Path::new("/dev/nvidia0-uvm")));
        }

        #[test]
        fn test_edge_cases() {
            // Path variations
            assert!(!is_gpu_device(Path::new("/dev/nvi")));
            assert!(!is_gpu_device(Path::new("/home/nvidia0")));
            assert!(!is_gpu_device(Path::new("nvidia0")));
            assert!(!is_gpu_device(Path::new("/dev/nvidia 0")));
            assert!(!is_gpu_device(Path::new("/dev/NVIDIA0")));
        }
    }
}
