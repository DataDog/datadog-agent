// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod helpers;

use helpers::{TestEnv, pid_is_alive};

// ===========================================================================
// Group 1: Connectivity and Basic CLI
// ===========================================================================

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
