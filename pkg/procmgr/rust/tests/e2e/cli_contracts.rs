// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{TestEnv, write_config};
use dd_procmgrd::test_helpers;

#[test]
fn test_cli_all_commands_json_parseable() {
    let env = TestEnv::new()
        .with_config("svc", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[svc] spawned");

    let commands: Vec<&[&str]> = vec![
        &["status", "--json"],
        &["list", "--json"],
        &["describe", "--json", "svc"],
        &["config", "--json"],
    ];
    for args in &commands {
        let out = env.cli(args);
        out.assert_success();
        out.stdout_json();
    }

    env.cli_stop_json("svc").assert_success().stdout_json();

    env.cli_start_json("svc").assert_success().stdout_json();

    env.create_sleep("dyn", &["--json", "--no-auto-start"])
        .assert_success()
        .stdout_json();

    write_config(
        env.config_dir(),
        "extra",
        &test_helpers::sleep_config_with("auto_start: false\n"),
    );
    env.cli_reload_json().assert_success().stdout_json();
}

#[test]
fn test_cli_errors_on_stderr() {
    let env = TestEnv::new().start();

    let cases: Vec<(&[&str], &str)> = vec![
        (&["describe", "nonexistent"], "not found"),
        (&["start", "nonexistent"], "not found"),
        (&["stop", "nonexistent"], "not found"),
    ];
    for (args, pattern) in &cases {
        let out = env.cli(args);
        out.assert_failure();
        out.assert_stderr_contains(pattern);
        assert!(
            out.stdout.trim().is_empty(),
            "stdout should be empty on error for {:?}, got: {}",
            args,
            out.stdout
        );
    }
}

#[test]
fn test_cli_exit_codes() {
    let env = TestEnv::new()
        .with_config("svc", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[svc] spawned");

    let success_cmds: Vec<&[&str]> =
        vec![&["status"], &["list"], &["describe", "svc"], &["config"]];
    for args in &success_cmds {
        let out = env.cli(args);
        assert_eq!(out.status.code(), Some(0), "expected exit 0 for {:?}", args);
    }

    let failure_cmds: Vec<&[&str]> = vec![
        &["describe", "nonexistent"],
        &["start", "nonexistent"],
        &["stop", "nonexistent"],
    ];
    for args in &failure_cmds {
        let out = env.cli(args);
        assert_eq!(out.status.code(), Some(1), "expected exit 1 for {:?}", args);
    }
}
