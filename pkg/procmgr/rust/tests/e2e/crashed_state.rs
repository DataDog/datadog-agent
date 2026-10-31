// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, StatusProcessesCount, TestEnv};

/// The `faulter` fixture dies from SIGSEGV on Unix and an access violation on
/// Windows, so it never returns a value and must not be reported as `Failed`.
#[test]
fn faulter_fixture_lands_in_crashed() {
    let procmgr = TestEnv::new().with_process("faulter").start();
    procmgr.assert_process_state_within("faulter", ProcessExpect::Crashed);
    procmgr
        .require_status()
        .assert_processes_count(StatusProcessesCount {
            total: Some(1),
            crashed: Some(1),
            failed: Some(0),
            exited: Some(0),
            running: Some(0),
            ..Default::default()
        });
}

/// The omission this whole change is most likely to make: if `Crashed` is left
/// out of the restart policy, a segfaulting child stops being supervised.
#[test]
fn restart_always_respawns_crashed() {
    let procmgr = TestEnv::new().with_process("faulter_always").start();
    procmgr.assert_restart_count_at_least("faulter_always", 2);
}

#[test]
fn restart_on_failure_respawns_crashed() {
    let procmgr = TestEnv::new().with_process("faulter_on_failure").start();
    procmgr.assert_restart_count_at_least("faulter_on_failure", 2);
}

#[test]
fn restart_on_success_leaves_crashed_terminal() {
    let procmgr = TestEnv::new().with_process("faulter_on_success").start();
    procmgr.assert_process_state_within("faulter_on_success", ProcessExpect::Crashed);
    assert_eq!(
        procmgr
            .process("faulter_on_success")
            .expect("snap")
            .restart_count,
        0,
        "on-success must not respawn an unsuccessful exit"
    );
}

/// The one test that proves the split rather than the rename: a crash and a
/// non-zero exit in the same catalog land in different counters.
#[test]
fn status_counts_crashed_separately_from_failed() {
    let procmgr = TestEnv::new()
        .with_process("faulter")
        .with_process("exit_fail")
        .start();
    procmgr.assert_process_state_within("faulter", ProcessExpect::Crashed);
    procmgr.assert_process_state_within("exit_fail", ProcessExpect::Failed);

    procmgr
        .require_status()
        .assert_processes_count(StatusProcessesCount {
            total: Some(2),
            crashed: Some(1),
            failed: Some(1),
            exited: Some(0),
            running: Some(0),
            ..Default::default()
        });
}
