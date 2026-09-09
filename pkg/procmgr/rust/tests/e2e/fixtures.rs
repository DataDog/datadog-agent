// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, StatusProcessesCount, TestEnv};

#[test]
fn sleeper_fixture_auto_starts() {
    let env = TestEnv::new().with_process("sleeper");
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        running: Some(1),
        created: Some(0),
        ..Default::default()
    });
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper", ProcessExpect::Running);
}

#[test]
fn sleeper_fixture_no_auto_start() {
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
}

#[test]
fn invalid_syntax_fixture_skipped() {
    let env = TestEnv::new()
        .with_process("sleeper")
        .with_process("invalid_syntax");
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        running: Some(1),
        created: Some(0),
        ..Default::default()
    });
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper", ProcessExpect::Running);
    list.assert_absent("invalid_syntax");
    procmgr.assert_config_skip_logged("invalid_syntax");
}

#[test]
fn missing_binary_fixture_fails_to_spawn() {
    let env = TestEnv::new().with_process("missing_binary");
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        failed: Some(1),
        running: Some(0),
        created: Some(0),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("missing_binary", ProcessExpect::Failed);
}

#[test]
fn exit_ok_fixture_exits_cleanly() {
    let env = TestEnv::new().with_process("exit_ok");
    let procmgr = env.start();
    procmgr.assert_process_state_within("exit_ok", ProcessExpect::Exited);
    let status = procmgr.require_status();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        exited: Some(1),
        running: Some(0),
        failed: Some(0),
        ..Default::default()
    });
}

#[test]
fn exit_fail_fixture_exits_with_failure() {
    let env = TestEnv::new().with_process("exit_fail");
    let procmgr = env.start();
    procmgr.assert_process_state_within("exit_fail", ProcessExpect::Failed);
    let status = procmgr.require_status();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        failed: Some(1),
        running: Some(0),
        exited: Some(0),
        ..Default::default()
    });
}

#[test]
fn condition_blocked_fixture_stays_created() {
    let env = TestEnv::new().with_process("condition_blocked");
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
    list.assert_process_state("condition_blocked", ProcessExpect::Created);
    procmgr.assert_condition_path_not_met_logged(
        "condition_blocked",
        "/nonexistent/path/procmgr-condition-test",
    );
}

#[test]
fn test_cli_condition_path_exists_not_met() {
    let env = TestEnv::new()
        .with_config(
            "missing-bin",
            "command: /nonexistent/binary\ncondition_path_exists: /nonexistent/binary\n",
        )
        .start();

    env.daemon()
        .wait_for_log_default("[missing-bin] condition_path_exists not met");

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["name"], "missing-bin");
    assert_eq!(json[0]["state"], "Created");
    assert_eq!(json[0]["pid"], 0);
}
