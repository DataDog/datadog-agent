// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{
    DescribeExpect, ProcessExpect, TestEnv, expected_agent_spawn_user,
    expected_runtime_user_for_pid, pid_is_alive,
};
use dd_procmgrd::test_helpers;
use std::collections::BTreeMap;

#[test]
fn describe_running_process_matches_fixture() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let list_snap = procmgr.process("sleeper").expect("sleeper");
    let pid = list_snap.pid as u32;
    let expected_user = expected_agent_spawn_user();
    let expected_runtime_user = expected_runtime_user_for_pid(pid);

    procmgr.assert_describe_matches(
        "sleeper",
        DescribeExpect {
            name: Some("sleeper".into()),
            state: Some("Running".into()),
            command: Some(list_snap.command),
            args: Some(list_snap.args),
            has_uuid: Some(true),
            pid_alive: Some(true),
            profile: Some("agent".into()),
            user: Some(expected_user),
            runtime_user: Some(expected_runtime_user),
            ..Default::default()
        },
    );
}

#[test]
fn describe_resolves_uuid_prefix() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let uuid = procmgr.process("sleeper").expect("sleeper").uuid.clone();
    let prefix = uuid[..8].to_string();

    for id in [&prefix, uuid.as_str()] {
        procmgr.assert_describe_matches(
            id,
            DescribeExpect {
                name: Some("sleeper".into()),
                uuid: Some(uuid.clone()),
                ..Default::default()
            },
        );
    }
}

#[test]
fn describe_shows_static_config_fields() {
    let procmgr = TestEnv::new().with_process("full").start();
    procmgr
        .wait_for_process_running("full")
        .expect("expected full running");

    let root = procmgr.env_root().display().to_string();
    procmgr.assert_describe_matches(
        "full",
        DescribeExpect {
            name: Some("full".into()),
            state: Some("Running".into()),
            description: Some("a test process".into()),
            working_dir: Some(root),
            restart_policy: Some("always".into()),
            auto_start: Some(true),
            env_contains: Some(BTreeMap::from([("MY_VAR".into(), "hello".into())])),
            after: Some(vec!["other".into()]),
            has_stdout_path: Some(true),
            has_stderr_path: Some(true),
            ..Default::default()
        },
    );
}

#[test]
fn describe_last_exit_text() {
    let env = TestEnv::new().with_process("exit_fail").start();
    env.assert_process_state_within("exit_fail", ProcessExpect::Failed);

    env.assert_describe_matches(
        "exit_fail",
        DescribeExpect {
            name: Some("exit_fail".into()),
            state: Some("Failed".into()),
            pid: Some(0),
            last_exit_code: Some(Some(1)),
            ..Default::default()
        },
    );

    env.cli_describe("exit_fail")
        .assert_success()
        .assert_field("Last Exit", "exit 1");
}

#[test]
fn describe_restarts_text() {
    let env = TestEnv::new().with_process("crasher").start();
    env.assert_restart_count_at_least("crasher", 2);

    env.cli_describe("crasher")
        .assert_success()
        .assert_field("Name", "crasher")
        .assert_field_at_least("Restarts", 2);
}

#[test]
fn describe_not_found() {
    let env = TestEnv::new().start();

    env.cli_describe("nonexistent")
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn describe_json() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli_describe_json("sleeper");
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["name"], "sleeper");
    assert_eq!(json["state"], "Running");
    assert_eq!(json["command"], test_helpers::sleep_cmd(300).0);
    assert_eq!(json["args"], test_helpers::sleep_args_json());
    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());

    let pid = json["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0);
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}
