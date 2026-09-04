// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{TestEnv, pid_is_alive, wait_for_pid_gone};
use dd_procmgrd::test_helpers;
use std::time::Duration;

#[test]
fn test_cli_create_minimal() {
    let env = TestEnv::new().start();

    let out = env.create_sleep("foo", &[]);
    out.assert_success().assert_has_field("UUID");

    env.daemon().wait_for_log_default("[foo] spawned");

    let pid = env.cli_describe("foo").pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_create_with_auto_start() {
    let env = TestEnv::new().start();

    env.create_sleep("svc", &[]).assert_success();

    env.daemon().wait_for_log_default("[svc] spawned");

    env.cli_list()
        .assert_success()
        .assert_table_row("svc", &[("STATE", "Running")]);

    let pid = env.cli_list().pid_from_table_row("svc");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_create_no_auto_start() {
    let env = TestEnv::new().start();

    env.create_sleep("manual", &["--no-auto-start"])
        .assert_success();

    env.cli_list()
        .assert_success()
        .assert_table_row("manual", &[("STATE", "Created"), ("PID", "-")]);
}

#[test]
fn test_cli_create_with_all_options() {
    let env = TestEnv::new()
        .with_config("dep", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[dep] spawned");

    let tmp = test_helpers::temp_dir_str();
    env.create_sleep(
        "full",
        &[
            "--env",
            "KEY1=val1",
            "--env",
            "KEY2=val2",
            "--working-dir",
            &tmp,
            "--restart-policy",
            "always",
            "--description",
            "full test",
            "--after",
            "dep",
        ],
    )
    .assert_success();

    env.daemon().wait_for_log_default("[full] spawned");

    let out = env.cli_describe("full");
    out.assert_success()
        .assert_field("Name", "full")
        .assert_field("Command", test_helpers::sleep_cmd(300).0)
        .assert_field("Working Dir", &tmp)
        .assert_field("Restart Policy", "always")
        .assert_field("Description", "full test");
}

#[test]
fn test_cli_create_then_describe() {
    let env = TestEnv::new().start();

    env.create_sleep("svc", &["--no-auto-start"])
        .assert_success();

    let out = env.cli_describe("svc");
    out.assert_success()
        .assert_field("Name", "svc")
        .assert_field("State", "Created")
        .assert_field("Command", test_helpers::sleep_cmd(300).0)
        .assert_field("Args", &test_helpers::sleep_args_display())
        .assert_field("PID", "-")
        .assert_has_field("UUID");
}

#[test]
fn test_cli_create_duplicate_name() {
    let env = TestEnv::new().start();

    env.create_sleep("dup", &[]).assert_success();

    env.daemon().wait_for_log_default("[dup] spawned");

    env.create_sleep("dup", &[])
        .assert_failure()
        .assert_stderr_contains("already exists");
}

#[test]
fn test_cli_create_empty_command() {
    let env = TestEnv::new().start();

    env.cli(&["create", "--name", "foo", "--command", ""])
        .assert_failure()
        .assert_stderr_contains("command must not be empty");
}

#[test]
fn test_cli_create_invalid_name() {
    let env = TestEnv::new().start();

    env.create_sleep("bad name!", &[])
        .assert_failure()
        .assert_stderr_contains("name must only contain");
}

#[test]
fn test_cli_create_json() {
    let env = TestEnv::new().start();

    let out = env.create_sleep("svc", &["--json"]);
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["name"], "svc");
    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
}

#[test]
fn test_cli_create_env_vars() {
    let env = TestEnv::new().start();

    env.create_sleep(
        "env-svc",
        &["--env", "FOO=bar", "--env", "BAZ=qux", "--no-auto-start"],
    )
    .assert_success();

    let out = env.cli_describe_json("env-svc");
    out.assert_success();
    let json = out.stdout_json();

    let env_map = json["env"].as_object().expect("env should be an object");
    assert_eq!(env_map.get("FOO").and_then(|v| v.as_str()), Some("bar"));
    assert_eq!(env_map.get("BAZ").and_then(|v| v.as_str()), Some("qux"));
}

#[test]
fn test_cli_create_with_dependencies() {
    let env = TestEnv::new().start();

    env.create_sleep("backend", &["--no-auto-start"])
        .assert_success();

    env.create_sleep("frontend", &["--after", "backend", "--no-auto-start"])
        .assert_success();

    env.cli_list()
        .assert_success()
        .assert_table_row_count(2)
        .assert_table_row("backend", &[("STATE", "Created"), ("PID", "-")])
        .assert_table_row("frontend", &[("STATE", "Created"), ("PID", "-")]);

    env.cli_start("backend").assert_success();
    env.daemon().wait_for_log_default("[backend] spawned");
    env.cli_start("frontend").assert_success();
    env.daemon().wait_for_log_default("[frontend] spawned");

    let backend_pid = env.cli_list().pid_from_table_row("backend");
    let frontend_pid = env.cli_list().pid_from_table_row("frontend");
    assert!(
        pid_is_alive(backend_pid),
        "backend PID {backend_pid} should be alive"
    );
    assert!(
        pid_is_alive(frontend_pid),
        "frontend PID {frontend_pid} should be alive"
    );

    let out = env.cli_describe_json("frontend");
    out.assert_success();
    let json = out.stdout_json();
    let after = json["after"].as_array().expect("after should be an array");
    assert!(
        after.iter().any(|v| v.as_str() == Some("backend")),
        "frontend should depend on backend: {json}"
    );
}

#[test]
fn test_cli_create_nonexistent_dependency_ignored() {
    let env = TestEnv::new().start();

    let out = env.create_sleep("svc", &["--after", "does-not-exist"]);
    out.assert_success();
    out.assert_stderr_contains("not found, ignoring");

    env.daemon().wait_for_log_default("[svc] spawned");

    let pid = env.cli_list().pid_from_table_row("svc");
    assert!(
        pid_is_alive(pid),
        "PID {pid} should be alive despite missing dep"
    );

    let out = env.cli_describe_json("svc");
    out.assert_success();
    let json = out.stdout_json();
    let after = json["after"].as_array().expect("after should be an array");
    assert!(
        after.iter().any(|v| v.as_str() == Some("does-not-exist")),
        "after should still list the nonexistent dep: {json}"
    );
}

#[test]
fn test_cli_create_nonexistent_dependency_json_warnings() {
    let env = TestEnv::new().start();

    let out = env.create_sleep("svc", &["--after", "ghost", "--json"]);
    out.assert_success();
    let json = out.stdout_json();
    let warnings = json["warnings"]
        .as_array()
        .expect("warnings should be an array");
    assert!(
        warnings.iter().any(|w| {
            let s = w.as_str().unwrap_or("");
            s.contains("ghost") && s.contains("not found")
        }),
        "JSON warnings should mention missing dep: {json}"
    );
}

#[test]
fn test_cli_full_lifecycle() {
    let env = TestEnv::new().start();

    env.create_sleep("svc", &["--no-auto-start"])
        .assert_success();

    env.cli_list()
        .assert_success()
        .assert_table_row("svc", &[("STATE", "Created"), ("PID", "-")]);

    env.cli_start("svc").assert_success();
    env.daemon().wait_for_log_default("[svc] spawned");

    let out = env.cli_describe("svc");
    out.assert_success().assert_field("State", "Running");
    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");

    env.cli_stop("svc").assert_success();

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "PID {pid} should be gone after stop"
    );

    env.cli_describe("svc")
        .assert_success()
        .assert_field("State", "Stopped")
        .assert_field("PID", "-");
}

#[test]
fn test_cli_create_stop_start_cycle() {
    let env = TestEnv::new().start();

    env.create_sleep("svc", &[]).assert_success();
    env.daemon().wait_for_log_default("[svc] spawned");

    env.cli_describe("svc")
        .assert_success()
        .assert_field("State", "Running");

    env.cli_stop("svc").assert_success();
    env.cli_describe("svc")
        .assert_success()
        .assert_field("State", "Stopped");

    env.cli_start("svc").assert_success();
    env.daemon()
        .wait_for_log_count("[svc] spawned", 2, Duration::from_secs(10));
    env.cli_describe("svc")
        .assert_success()
        .assert_field("State", "Running");

    let pid = env.cli_describe("svc").pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive after restart");

    env.cli_stop("svc").assert_success();
    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "PID {pid} should be gone after second stop"
    );

    env.cli_start("svc").assert_success();
    env.daemon()
        .wait_for_log_count("[svc] spawned", 3, Duration::from_secs(10));

    let new_pid = env.cli_describe("svc").pid_from_field("PID");
    assert!(pid_is_alive(new_pid), "PID {new_pid} should be alive");
}
