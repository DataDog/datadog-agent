// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]
#![allow(clippy::cast_possible_wrap)]

use std::fs;
use std::process::Command;
use std::thread;
use std::time::Duration;
use tempfile::TempDir;

const SYSTEM_PROBE_LITE_BIN: &str = env!("CARGO_BIN_EXE_system-probe-lite");

#[test]
fn test_pid_file_created_and_cleaned_up_on_sigterm() {
    let temp_dir = TempDir::new().unwrap();
    let pid_path = temp_dir.path().join("system-probe-lite.pid");
    let socket_path = temp_dir.path().join("sysprobe.sock");

    let log_path = temp_dir.path().join("system-probe.log");

    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-level")
        .arg("info")
        .arg("--log-file")
        .arg(&log_path)
        .arg("--pid")
        .arg(&pid_path)
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start and create PID file
    thread::sleep(Duration::from_millis(500));

    // Verify PID file was created
    assert!(pid_path.exists(), "PID file should be created");

    // Verify PID file contains the correct PID
    let pid_content = fs::read_to_string(&pid_path).expect("Failed to read PID file");
    let file_pid: u32 = pid_content.trim().parse().expect("Invalid PID in file");
    assert_eq!(file_pid, child.id(), "PID file should contain process ID");

    // Send SIGTERM to the process
    #[cfg(unix)]
    {
        use nix::sys::signal::{self, Signal};
        use nix::unistd::Pid;
        signal::kill(Pid::from_raw(child.id() as i32), Signal::SIGTERM)
            .expect("Failed to send SIGTERM");
    }

    // Wait for process to exit
    let status = child.wait().expect("Failed to wait on child");
    assert!(
        status.success() || status.code() == Some(0),
        "Process should exit cleanly"
    );

    // Give it a moment for cleanup to complete
    thread::sleep(Duration::from_millis(100));

    // Verify PID file was cleaned up
    assert!(!pid_path.exists(), "PID file should be removed on SIGTERM");
}

#[test]
fn test_pid_file_created_and_cleaned_up_on_sigint() {
    let temp_dir = TempDir::new().unwrap();
    let pid_path = temp_dir.path().join("system-probe-lite.pid");
    let socket_path = temp_dir.path().join("sysprobe.sock");
    let log_path = temp_dir.path().join("system-probe.log");

    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-level")
        .arg("info")
        .arg("--log-file")
        .arg(&log_path)
        .arg("--pid")
        .arg(&pid_path)
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start and create PID file
    thread::sleep(Duration::from_millis(500));

    // Verify PID file was created
    assert!(pid_path.exists(), "PID file should be created");

    // Verify PID file contains the correct PID
    let pid_content = fs::read_to_string(&pid_path).expect("Failed to read PID file");
    let file_pid: u32 = pid_content.trim().parse().expect("Invalid PID in file");
    assert_eq!(file_pid, child.id(), "PID file should contain process ID");

    // Send SIGINT (Ctrl+C) to the process
    #[cfg(unix)]
    {
        use nix::sys::signal::{self, Signal};
        use nix::unistd::Pid;
        signal::kill(Pid::from_raw(child.id() as i32), Signal::SIGINT)
            .expect("Failed to send SIGINT");
    }

    // Wait for process to exit
    let status = child.wait().expect("Failed to wait on child");
    assert!(
        status.success() || status.code() == Some(0),
        "Process should exit cleanly"
    );

    // Give it a moment for cleanup to complete
    thread::sleep(Duration::from_millis(100));

    // Verify PID file was cleaned up
    assert!(!pid_path.exists(), "PID file should be removed on SIGINT");
}

#[test]
fn test_no_pid_file_without_flag() {
    let temp_dir = TempDir::new().unwrap();
    let pid_path = temp_dir.path().join("system-probe-lite.pid");
    let socket_path = temp_dir.path().join("sysprobe.sock");

    // Spawn system-probe-lite WITHOUT --pid flag
    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-level")
        .arg("info")
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start
    thread::sleep(Duration::from_millis(500));

    // Verify PID file was NOT created
    assert!(
        !pid_path.exists(),
        "PID file should not be created without --pid flag"
    );

    // Clean shutdown
    child.kill().ok();
    child.wait().expect("Failed to wait on child");
}
