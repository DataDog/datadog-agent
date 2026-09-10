// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{DescribeExpect, ProcessExpect, TestEnv};
use std::time::Duration;

#[test]
fn restart_always_increments_count_in_list() {
    let procmgr = TestEnv::new().with_process("crasher").start();
    procmgr.assert_restart_count_at_least("crasher", 2);
    procmgr.require_list().assert_len(1);
}

#[test]
fn describe_shows_restart_count_after_always_policy() {
    let procmgr = TestEnv::new().with_process("crasher").start();
    procmgr.assert_restart_count_at_least("crasher", 2);
    procmgr.assert_describe_matches(
        "crasher",
        DescribeExpect {
            name: Some("crasher".into()),
            restart_count_at_least: Some(2),
            ..Default::default()
        },
    );
}

#[test]
fn restart_on_failure_leaves_clean_exit_terminal() {
    let procmgr = TestEnv::new().with_process("exit_ok_on_failure").start();
    procmgr.assert_process_state_within("exit_ok_on_failure", ProcessExpect::Exited);
    assert_eq!(
        procmgr
            .process("exit_ok_on_failure")
            .expect("snap")
            .restart_count,
        0
    );
}

#[test]
fn restart_on_success_respawns_clean_exit() {
    let procmgr = TestEnv::new().with_process("exit_ok_on_success").start();
    procmgr.assert_restart_count_at_least("exit_ok_on_success", 2);
}

#[test]
fn restart_on_success_leaves_failed_exit_terminal() {
    let procmgr = TestEnv::new().with_process("exit_fail_on_success").start();
    procmgr.assert_process_state_within("exit_fail_on_success", ProcessExpect::Failed);
    assert_eq!(
        procmgr
            .process("exit_fail_on_success")
            .expect("snap")
            .restart_count,
        0
    );
}

#[test]
fn restart_on_failure_respawns_failed_exit() {
    let procmgr = TestEnv::new().with_process("exit_fail_on_failure").start();
    procmgr.assert_restart_count_at_least("exit_fail_on_failure", 2);
}

#[test]
fn burst_limit_stops_with_failed_state() {
    let procmgr = TestEnv::new().with_process("burst_limited").start();
    procmgr
        .wait_for_restart_count_terminal(
            "burst_limited",
            4,
            ProcessExpect::Failed,
            Duration::from_secs(3),
            Duration::from_secs(30),
        )
        .unwrap_or_else(|e| panic!("expected burst_limited to hit restart burst limit: {e}"));
}

#[test]
fn burst_interval_allows_continued_restarts() {
    let procmgr = TestEnv::new().with_process("burst_spaced").start();
    procmgr
        .wait_for_restart_count_at_least("burst_spaced", 5, Duration::from_secs(10))
        .unwrap_or_else(|e| panic!("expected spaced restarts to continue: {e}"));
}
