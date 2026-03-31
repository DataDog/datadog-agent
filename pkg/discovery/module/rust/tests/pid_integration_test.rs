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
fn test_pid_file_cleaned_up_on_sigterm() {
    let temp_dir = TempDir::new().unwrap();
    let pid_path = temp_dir.path().join("system-probe-lite.pid");
    let socket_path = temp_dir.path().join("sysprobe.sock");
    let log_path = temp_dir.path().join("system-probe.log");

    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("run")
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-file")
        .arg(&log_path)
        .arg("--pid")
        .arg(&pid_path)
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start
    thread::sleep(Duration::from_millis(500));

    // Simulate the PID file that Go's pid component would have written before exec.
    // In production, the Go process writes this file; system-probe-lite only cleans it up.
    fs::write(&pid_path, child.id().to_string()).expect("Failed to create PID file");
    assert!(pid_path.exists(), "PID file should exist before signal");

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
fn test_pid_file_cleaned_up_on_sigint() {
    let temp_dir = TempDir::new().unwrap();
    let pid_path = temp_dir.path().join("system-probe-lite.pid");
    let socket_path = temp_dir.path().join("sysprobe.sock");
    let log_path = temp_dir.path().join("system-probe.log");

    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("run")
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-file")
        .arg(&log_path)
        .arg("--pid")
        .arg(&pid_path)
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start
    thread::sleep(Duration::from_millis(500));

    // Simulate the PID file that Go's pid component would have written before exec.
    fs::write(&pid_path, child.id().to_string()).expect("Failed to create PID file");
    assert!(pid_path.exists(), "PID file should exist before signal");

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
