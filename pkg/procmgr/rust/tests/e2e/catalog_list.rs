// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, TestEnv, pid_is_alive};
use dd_procmgrd::test_helpers;

#[test]
fn list_empty_when_no_processes() {
    let procmgr = TestEnv::new().start();
    procmgr.require_status().assert_ready();
    procmgr.require_list().assert_empty();
}

#[test]
fn list_shows_running_and_created_mix() {
    let env = TestEnv::new()
        .with_process("sleeper")
        .with_process("sleeper_idle");
    let procmgr = env.start();
    procmgr.require_status().assert_ready();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(2);
    list.assert_process_state("sleeper", ProcessExpect::Running);
    list.assert_process_state("sleeper_idle", ProcessExpect::Created);
    for name in ["sleeper", "sleeper_idle"] {
        assert_eq!(
            list.require_process(name).user,
            test_helpers::expected_spawn_user(name),
            "list user for {name}"
        );
    }
}

#[test]
fn list_shows_exited_with_last_exit_code() {
    let env = TestEnv::new()
        .with_process("exit_ok")
        .with_process("exit_fail");
    let procmgr = env.start();
    procmgr.assert_process_state_within("exit_ok", ProcessExpect::Exited);
    procmgr.assert_process_state_within("exit_fail", ProcessExpect::Failed);
    let list = procmgr.require_list();
    list.assert_len(2);
    list.assert_last_exit_code("exit_ok", 0);
    list.assert_last_exit_code("exit_fail", 1);
}

#[test]
fn test_cli_list_terminal_table_fields() {
    let procmgr = TestEnv::new()
        .with_process("exit_ok")
        .with_process("exit_fail")
        .start();
    procmgr.assert_process_state_within("exit_ok", ProcessExpect::Exited);
    procmgr.assert_process_state_within("exit_fail", ProcessExpect::Failed);

    let python = test_helpers::python_exe();
    procmgr
        .cli_list()
        .assert_success()
        .assert_table_row(
            "exit_ok",
            &[
                ("STATE", "Exited"),
                ("PID", "-"),
                ("LAST EXIT", "exit 0"),
                ("PROFILE", "agent"),
                ("USER", &test_helpers::expected_spawn_user("exit_ok")),
                ("COMMAND", &python),
            ],
        )
        .assert_table_row(
            "exit_fail",
            &[
                ("STATE", "Failed"),
                ("PID", "-"),
                ("LAST EXIT", "exit 1"),
                ("PROFILE", "agent"),
                ("USER", &test_helpers::expected_spawn_user("exit_fail")),
                ("COMMAND", &python),
            ],
        )
        .assert_table_row_count(2);
}

#[test]
fn test_cli_list_json() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli_list_json();
    out.assert_success();
    let json = out.stdout_json();
    let arr = json.as_array().expect("expected JSON array");
    assert_eq!(arr.len(), 1);

    let entry = &arr[0];
    assert_eq!(entry["name"], "sleeper");
    assert_eq!(entry["state"], "Running");
    assert_eq!(entry["profile"], "agent");
    assert_eq!(entry["user"], test_helpers::expected_spawn_user("sleeper"));
    assert_eq!(entry["command"], test_helpers::sleep_cmd(300).0);
    assert_eq!(entry["args"], test_helpers::sleep_args_json());
    assert_eq!(entry["restart_count"], 0);
    assert!(!entry["uuid"].as_str().unwrap_or("").is_empty());
    assert!(entry["last_exit_code"].is_null());
    assert!(entry["last_signal"].is_null());

    let pid = entry["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0, "running process should have a PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_list_json_empty() {
    let env = TestEnv::new().start();

    let out = env.cli_list_json();
    out.assert_success();
    let json = out.stdout_json();
    let arr = json.as_array().expect("expected JSON array");
    assert!(arr.is_empty(), "expected empty array, got {json}");
}
