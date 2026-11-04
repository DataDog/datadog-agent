// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, StatusProcessesCount, TestEnv, write_config};
use dd_procmgrd::test_helpers;

fn write_agent_yaml(env: &TestEnv, process_collection_enabled: bool) {
    write_config(
        env.env_root(),
        "datadog",
        &format!(
            "process_config:\n  process_collection:\n    enabled: {process_collection_enabled}\n"
        ),
    );
}

#[test]
fn auto_start_false_is_skipped() {
    let procmgr = TestEnv::new().with_process("sleeper_idle").start();
    let list = procmgr.require_list();
    list.assert_process_state("sleeper_idle", ProcessExpect::Skipped);
    list.assert_skip_reasons("sleeper_idle", &["auto_start_false"]);
    assert_ne!(
        list.require_process("sleeper_idle").state,
        "Created",
        "a start-hold must not rest in Created after GetStatus.ready"
    );

    procmgr.assert_start_process("sleeper_idle");
    procmgr
        .require_list()
        .assert_process_state("sleeper_idle", ProcessExpect::Running);
}

#[test]
fn config_veto_is_skipped() {
    let env = TestEnv::new();
    write_agent_yaml(&env, true);
    let procmgr = env.with_process("config_veto").start();

    let list = procmgr.require_list();
    list.assert_process_state("config_veto", ProcessExpect::Skipped);
    list.assert_skip_reasons("config_veto", &["config_veto"]);
}

#[test]
fn config_veto_open_auto_starts_process() {
    let env = TestEnv::new();
    write_agent_yaml(&env, false);
    let procmgr = env.with_process("config_veto").start();
    procmgr
        .wait_for_process_running("config_veto")
        .expect("expected config_veto running when the veto is open");
}

#[test]
fn reasons_combine_auto_start_and_closed_gate() {
    let env = TestEnv::new();
    write_agent_yaml(&env, false);
    let procmgr = env.with_process("config_gate_no_auto_start").start();

    let list = procmgr.require_list();
    list.assert_process_state("config_gate_no_auto_start", ProcessExpect::Skipped);
    list.assert_skip_reasons(
        "config_gate_no_auto_start",
        &["auto_start_false", "config_gate"],
    );
}

#[test]
fn cycle_is_skipped_ordering() {
    let extra_a = "after:\n  - cyc-b\n";
    let extra_b = "after:\n  - cyc-a\n";
    let procmgr = TestEnv::new()
        .with_config("cyc-a", &test_helpers::sleep_config_with(extra_a))
        .with_config("cyc-b", &test_helpers::sleep_config_with(extra_b))
        .start();

    let list = procmgr.require_list();
    list.assert_process_state("cyc-a", ProcessExpect::Skipped);
    list.assert_process_state("cyc-b", ProcessExpect::Skipped);
    list.assert_skip_reasons("cyc-a", &["ordering"]);
    list.assert_skip_reasons("cyc-b", &["ordering"]);

    procmgr.assert_reload_matches(Default::default());
    let list = procmgr.require_list();
    list.assert_process_state("cyc-a", ProcessExpect::Skipped);
    list.assert_process_state("cyc-b", ProcessExpect::Skipped);
}

#[test]
fn never_skipped_after_child() {
    let procmgr = TestEnv::new().with_process("sleeper_idle").start();
    procmgr.assert_start_process("sleeper_idle");
    procmgr.assert_stop_process("sleeper_idle");
    procmgr
        .require_list()
        .assert_process_state("sleeper_idle", ProcessExpect::Stopped);

    procmgr.assert_reload_matches(Default::default());
    procmgr
        .require_list()
        .assert_process_state("sleeper_idle", ProcessExpect::Stopped);
}

#[test]
fn ready_created_is_empty_for_holds() {
    let procmgr = TestEnv::new()
        .with_process("sleeper")
        .with_process("sleeper_idle")
        .start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(2),
        running: Some(1),
        skipped: Some(1),
        created: Some(0),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_process_state("sleeper", ProcessExpect::Running);
    list.assert_process_state("sleeper_idle", ProcessExpect::Skipped);
}
