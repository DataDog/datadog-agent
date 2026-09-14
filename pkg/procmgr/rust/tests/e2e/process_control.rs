// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{
    ProcessExpect, StatusProcessesCount, TestEnv, kill_pid_force, pid_is_alive, wait_for_pid_gone,
};
use dd_procmgrd::test_helpers;
use std::time::Duration;

#[test]
fn start_process_transitions_created_to_running() {
    let env = TestEnv::new().with_process("sleeper_idle");
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        created: Some(1),
        running: Some(0),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper_idle", ProcessExpect::Created);

    procmgr.assert_start_process("sleeper_idle");

    let status = procmgr.require_status();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        running: Some(1),
        created: Some(0),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper_idle", ProcessExpect::Running);
}

#[test]
fn stop_process_transitions_running_to_stopped() {
    let env = TestEnv::new().with_process("sleeper");
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        running: Some(1),
        stopped: Some(0),
        created: Some(0),
        ..Default::default()
    });
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    procmgr.assert_stop_process("sleeper");

    let status = procmgr.require_status();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        stopped: Some(1),
        running: Some(0),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper", ProcessExpect::Stopped);
}

#[test]
fn stop_process_kills_child() {
    let env = TestEnv::new().with_process("sleeper");
    let procmgr = env.start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let snapshot = procmgr
        .process("sleeper")
        .expect("sleeper should be listed");
    let pid = snapshot.pid as u32;
    assert!(pid > 0, "sleeper should have a PID, got {snapshot:?}");
    assert!(pid_is_alive(pid), "PID {pid} should be alive before stop");

    procmgr.assert_stop_process("sleeper");

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "child PID {pid} should be gone after stop"
    );
}

#[test]
fn test_cli_start_by_uuid() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            &test_helpers::sleep_config_with("auto_start: false\n"),
        )
        .start();

    let list_json = env.cli_list_json().stdout_json();
    let uuid = list_json[0]["uuid"]
        .as_str()
        .expect("uuid should be a string");
    let prefix = &uuid[..8];

    env.cli_start(prefix)
        .assert_success()
        .assert_field("State", "Running");

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli_describe("sleeper").pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_start_already_running() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli_start("sleeper")
        .assert_failure()
        .assert_stderr_contains("already");
}

#[test]
fn test_cli_start_not_found() {
    let env = TestEnv::new().start();

    env.cli_start("nonexistent")
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn test_cli_start_json() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            &test_helpers::sleep_config_with("auto_start: false\n"),
        )
        .start();

    let out = env.cli_start_json("sleeper");
    out.assert_success();
    let json = out.stdout_json();

    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
    assert_eq!(json["state"], "Running");

    let pid = json["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0, "started process should have a PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_stop_by_uuid() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let list_json = env.cli_list_json().stdout_json();
    let uuid = list_json[0]["uuid"]
        .as_str()
        .expect("uuid should be a string");
    let prefix = &uuid[..8];

    env.cli_stop(prefix)
        .assert_success()
        .assert_field("State", "Stopped");
}

#[test]
fn test_cli_stop_already_stopped() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli_stop("sleeper").assert_success();
    env.daemon().wait_for_log_default("[sleeper] stopped");

    env.cli_stop("sleeper")
        .assert_failure()
        .assert_stderr_contains("not running");
}

#[test]
fn test_cli_stop_not_found() {
    let env = TestEnv::new().start();

    env.cli_stop("nonexistent")
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn test_cli_stop_json() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli_stop_json("sleeper");
    out.assert_success();
    let json = out.stdout_json();

    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
    assert_eq!(json["state"], "Stopped");
}

#[test]
fn test_cli_stop_start_then_kill_restarts_on_failure() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            &test_helpers::sleep_config_with("restart: on-failure\n"),
        )
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli_stop("sleeper").assert_success();
    env.cli_start("sleeper").assert_success();
    assert!(
        env.daemon()
            .wait_for_log_count("[sleeper] spawned", 2, Duration::from_secs(10)),
        "start after stop should spawn a new child"
    );

    let pid = env.cli_describe("sleeper").pid_from_field("PID");
    kill_pid_force(pid);

    assert!(
        env.daemon()
            .wait_for_log_count("[sleeper] spawned", 3, Duration::from_secs(10)),
        "on-failure should auto-restart after stop -> start -> external kill"
    );

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["state"], "Running");
    assert!(
        json[0]["restart_count"].as_u64().unwrap() >= 1,
        "restart_count should reflect the crash restart"
    );
}
