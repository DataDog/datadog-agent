// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod helpers;

use helpers::{pid_is_alive, CliRunner, TestEnv};
use std::path::Path;

#[test]
fn test_cli_daemon_starts_ok() {
    let env = TestEnv::new().start();

    assert!(
        pid_is_alive(env.daemon_pid()),
        "daemon process should be alive"
    );

    env.cli(&["status"])
        .assert_success()
        .assert_field("Ready", "true")
        .assert_has_field("Version")
        .assert_has_field("Uptime");
}

#[test]
fn test_cli_fails_when_daemon_not_running() {
    let env = TestEnv::new();

    env.cli(&["status"])
        .assert_failure()
        .assert_stderr_contains("Error");
}

#[test]
fn test_cli_fails_with_invalid_socket() {
    let runner = CliRunner::new(Path::new("/nonexistent/daemon.sock"));

    runner
        .run(&["status"])
        .assert_failure()
        .assert_stderr_contains("Error");
}

#[test]
fn test_cli_config_basic() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    let config_dir = env.config_dir().display().to_string();

    env.cli(&["config"])
        .assert_success()
        .assert_field("Source", "yaml")
        .assert_field("Location", &config_dir)
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "0");
}
