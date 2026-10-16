// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{DescribeExpect, TestEnv, wait_for_pid_gone};
use std::os::unix::process::ExitStatusExt;
use std::time::Duration;

const DAEMON_EXIT_TIMEOUT: Duration = Duration::from_secs(30);
const CRITICAL_YAML: &str = "exit_with_supervisor: true";

fn running_pid(env: &TestEnv, name: &str) -> u32 {
    let snapshot = env
        .process(name)
        .unwrap_or_else(|e| panic!("process '{name}' not listed: {e}"));
    assert!(
        snapshot.pid > 0,
        "process '{name}' has no pid: {snapshot:?}"
    );
    snapshot.pid as u32
}

#[test]
fn critical_exit_ends_the_daemon_with_the_child_code() {
    let mut procmgr = TestEnv::new()
        .with_process("critical_exit")
        .with_process("sleeper")
        .start();
    let sleeper_pid = running_pid(&procmgr, "sleeper");

    let status = procmgr
        .wait_for_daemon_exit(DAEMON_EXIT_TIMEOUT)
        .expect("daemon should exit after the critical process does");
    assert_eq!(
        status.code(),
        Some(42),
        "daemon must exit with the critical child's code, got {status:?}"
    );
    assert!(
        wait_for_pid_gone(sleeper_pid, Duration::from_secs(10)),
        "sibling process {sleeper_pid} must be stopped before the daemon exits"
    );
}

#[test]
fn critical_clean_exit_still_ends_the_daemon() {
    let mut procmgr = TestEnv::new()
        .with_process("critical_exit_ok")
        .with_process("sleeper")
        .start();
    let sleeper_pid = running_pid(&procmgr, "sleeper");

    let status = procmgr
        .wait_for_daemon_exit(DAEMON_EXIT_TIMEOUT)
        .expect("a clean critical exit must end the daemon too");
    assert_eq!(
        status.code(),
        Some(0),
        "exit code 0 must be propagated as is, got {status:?}"
    );
    assert!(
        wait_for_pid_gone(sleeper_pid, Duration::from_secs(10)),
        "sibling process {sleeper_pid} must be stopped before the daemon exits"
    );
}

#[test]
fn critical_flag_takes_precedence_over_restart() {
    let mut procmgr = TestEnv::new().with_process("critical_exit").start();
    procmgr
        .wait_for_daemon_exit(DAEMON_EXIT_TIMEOUT)
        .expect("daemon should exit after the critical process does");
    assert_eq!(
        procmgr.daemon().count_log_matches("restart #"),
        0,
        "a critical process must never be scheduled for restart: {:?}",
        procmgr.daemon().captured_logs()
    );
}

#[test]
fn auxiliary_crash_does_not_end_the_daemon() {
    let mut procmgr = TestEnv::new()
        .with_process("crasher")
        .with_process("sleeper")
        .start();
    procmgr.assert_restart_count_at_least("crasher", 2);
    assert!(
        procmgr.daemon_is_running(),
        "a non-critical process failing must leave the daemon up"
    );
}

#[test]
fn cli_stop_of_critical_ends_the_daemon() {
    let mut procmgr = TestEnv::new()
        .with_process_extra("sleeper", CRITICAL_YAML)
        .with_process("sleeper_long")
        .start();
    let aux_pid = running_pid(&procmgr, "sleeper_long");

    procmgr.cli_stop("sleeper").assert_success();

    let status = procmgr
        .wait_for_daemon_exit(DAEMON_EXIT_TIMEOUT)
        .expect("stopping the critical process must end the daemon");
    assert_eq!(
        status.code(),
        Some(0),
        "operator stop ends cleanly: {status:?}"
    );
    assert!(
        wait_for_pid_gone(aux_pid, Duration::from_secs(10)),
        "sibling process {aux_pid} must be stopped before the daemon exits"
    );
}

#[test]
fn daemon_shutdown_does_not_fire_critical_logic() {
    let mut procmgr = TestEnv::new()
        .with_process_extra("sleeper", CRITICAL_YAML)
        .with_process("sleeper_long")
        .start();
    let critical_pid = running_pid(&procmgr, "sleeper");

    let status = procmgr.stop_daemon();
    assert_eq!(
        status.code(),
        Some(0),
        "SIGTERM shutdown exits cleanly, got {status:?} (signal {:?})",
        status.signal()
    );
    assert!(wait_for_pid_gone(critical_pid, Duration::from_secs(10)));
    assert_eq!(
        procmgr.daemon().count_log_matches("critical process ended"),
        0,
        "shutdown must not be reported as a critical exit: {:?}",
        procmgr.daemon().captured_logs()
    );
}

#[test]
fn reload_replacing_a_critical_process_ends_the_daemon() {
    let mut procmgr = TestEnv::new()
        .with_process_extra("sleeper", CRITICAL_YAML)
        .with_process("sleeper_long")
        .start();
    let critical_pid = running_pid(&procmgr, "sleeper");

    procmgr.overwrite_with_fixture("sleeper", "sleeper_long");
    let _ = procmgr.cli_reload();

    let status = procmgr
        .wait_for_daemon_exit(DAEMON_EXIT_TIMEOUT)
        .expect("reload that stops the critical process must end the daemon");
    assert_eq!(status.code(), Some(0), "got {status:?}");
    assert!(wait_for_pid_gone(critical_pid, Duration::from_secs(10)));
}

#[test]
fn describe_reports_exit_with_supervisor() {
    let procmgr = TestEnv::new()
        .with_process_extra("sleeper", CRITICAL_YAML)
        .with_process("sleeper_long")
        .start();
    procmgr.assert_describe_matches(
        "sleeper",
        DescribeExpect {
            exit_with_supervisor: Some(true),
            ..Default::default()
        },
    );
    procmgr.assert_describe_matches(
        "sleeper_long",
        DescribeExpect {
            exit_with_supervisor: Some(false),
            ..Default::default()
        },
    );
    procmgr
        .cli_describe("sleeper")
        .assert_success()
        .assert_field("Exit w/ Supervisor", "true");
}
