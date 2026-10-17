// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{StatusProcessesCount, TestEnv, pid_is_alive, wait_for_pid_gone};
use dd_procmgrd::test_helpers;
use std::time::Duration;

#[test]
fn daemon_starts_ready() {
    let env = TestEnv::new();
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_version_not_empty();
    status.assert_processes_count(StatusProcessesCount::zeros());
}

#[test]
fn status_fails_without_daemon() {
    let env = TestEnv::new();
    env.status()
        .expect_err("expected status to fail without daemon");
}

#[test]
fn status_ready_without_config_dir() {
    let env = TestEnv::with_missing_config_dir();
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount::zeros());
}

#[test]
fn test_cli_daemon_nonexistent_config_dir() {
    let env = TestEnv::with_missing_config_dir();
    let config_dir = env.config_dir().display().to_string();
    let env = env.start();

    env.daemon().wait_for_log_default(&format!(
        "config directory {config_dir} does not exist, no processes to manage"
    ));

    env.cli_status()
        .assert_success()
        .assert_field("Ready", "true")
        .assert_field("Total Processes", "0");

    env.cli_list()
        .assert_success()
        .assert_stdout_contains("No processes");
}

#[test]
fn test_cli_daemon_shutdown_stops_children() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli_list().pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "child PID {pid} should be alive");

    drop(env);

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "child PID {pid} should be gone after daemon shutdown"
    );
}

#[cfg(unix)]
#[test]
fn test_cli_daemon_shutdown_via_sigint() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let child_pid = env.cli_list().pid_from_table_row("sleeper");
    let daemon_pid = env.daemon_pid();

    assert!(pid_is_alive(child_pid), "child should be alive");
    assert!(pid_is_alive(daemon_pid), "daemon should be alive");

    env.daemon().send_signal(nix::sys::signal::Signal::SIGINT);
    drop(env);

    assert!(
        !pid_is_alive(daemon_pid),
        "daemon PID {daemon_pid} should be gone after SIGINT"
    );
    assert!(
        !pid_is_alive(child_pid),
        "child PID {child_pid} should be gone after SIGINT shutdown"
    );
}
