// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, ReloadExpect, TestEnv, write_config};

/// Writes the `datadog.yaml` that the `config_gate` fixtures point their gate at.
fn write_agent_yaml(env: &TestEnv, process_collection_enabled: bool) {
    write_config(
        env.env_root(),
        "datadog",
        &format!(
            "process_config:\n  process_collection:\n    enabled: {process_collection_enabled}\n"
        ),
    );
}

fn write_system_probe_yaml(env: &TestEnv, network_enabled: bool) {
    write_config(
        env.env_root(),
        "system-probe",
        &format!("network_config:\n  enabled: {network_enabled}\n"),
    );
}

fn agent_yaml_path(env: &TestEnv) -> String {
    env.env_root().join("datadog.yaml").display().to_string()
}

#[test]
fn config_gate_open_auto_starts_process() {
    let env = TestEnv::new();
    write_agent_yaml(&env, true);
    let procmgr = env.with_process("config_gate").start();

    procmgr
        .wait_for_process_running("config_gate")
        .expect("expected config_gate running when the gate is open");
}

#[test]
fn config_gate_closed_stays_created() {
    let env = TestEnv::new();
    write_agent_yaml(&env, false);
    let procmgr = env.with_process("config_gate").start();

    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("config_gate", ProcessExpect::Created);
    procmgr.assert_config_gate_not_met_logged("config_gate", &agent_yaml_path(&procmgr));
}

/// Any single key across any gated file opens the gate, which is the shape the Windows
/// fleet template uses.
#[test]
fn config_gate_any_key_across_files_opens_gate() {
    let env = TestEnv::new();
    write_agent_yaml(&env, false);
    write_system_probe_yaml(&env, true);
    let procmgr = env.with_process("config_gate_multi").start();

    procmgr
        .wait_for_process_running("config_gate_multi")
        .expect("expected config_gate_multi running on the system-probe key alone");
}

#[test]
fn reload_starts_created_process_when_gate_opens() {
    let env = TestEnv::new();
    write_agent_yaml(&env, false);
    let procmgr = env.with_process("config_gate").start();
    procmgr
        .require_list()
        .assert_process_state("config_gate", ProcessExpect::Created);

    write_agent_yaml(&procmgr, true);
    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["config_gate".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        modified: Some(vec![]),
        ..Default::default()
    });

    procmgr
        .wait_for_process_running("config_gate")
        .expect("expected config_gate running once the gate opens on reload");
}

/// Reload re-evaluates gates, but `should_start` checks `auto_start` first, so an open
/// gate must not start a process the operator asked never to start on its own.
#[test]
fn reload_does_not_start_no_auto_start_process_with_open_gate() {
    let env = TestEnv::new();
    write_agent_yaml(&env, true);
    let procmgr = env.with_process("config_gate_no_auto_start").start();
    procmgr
        .require_list()
        .assert_process_state("config_gate_no_auto_start", ProcessExpect::Created);

    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["config_gate_no_auto_start".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        modified: Some(vec![]),
        ..Default::default()
    });

    procmgr
        .require_list()
        .assert_process_state("config_gate_no_auto_start", ProcessExpect::Created);
}

/// Blast radius of the gate feature on an ordinary process: reload must not resurrect a
/// process an operator stopped over RPC.
#[test]
fn reload_leaves_stopped_process_stopped() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    procmgr.assert_stop_process("sleeper");
    procmgr
        .require_list()
        .assert_process_state("sleeper", ProcessExpect::Stopped);

    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["sleeper".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        modified: Some(vec![]),
        ..Default::default()
    });

    procmgr
        .require_list()
        .assert_process_state("sleeper", ProcessExpect::Stopped);
}

/// Closing a gate does not stop a running process, matching legacy Windows SCM, which
/// reads config keys when it decides to start a dependent service rather than continuously.
#[test]
fn gate_closing_does_not_stop_running_process() {
    let env = TestEnv::new();
    write_agent_yaml(&env, true);
    let procmgr = env.with_process("config_gate").start();
    procmgr
        .wait_for_process_running("config_gate")
        .expect("expected config_gate running when the gate is open");

    write_agent_yaml(&procmgr, false);
    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["config_gate".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        modified: Some(vec![]),
        preserve_running_pids: vec!["config_gate".into()],
    });

    procmgr
        .require_list()
        .assert_process_state("config_gate", ProcessExpect::Running);
}
