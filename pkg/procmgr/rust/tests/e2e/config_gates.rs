// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, ReloadExpect, TestEnv};
use dd_procmgrd::test_helpers;
use std::fs;
use std::io::Write;
use std::path::Path;

fn write_agent_yaml(path: &Path, process_collection_enabled: bool) {
    let body = format!(
        "process_config:\n  process_collection:\n    enabled: {process_collection_enabled}\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n"
    );
    let mut file = fs::File::create(path).expect("create agent yaml");
    file.write_all(body.as_bytes()).expect("write agent yaml");
}

fn gated_sleeper_yaml(agent_yaml: &str) -> String {
    let (cmd, args) = test_helpers::sleep_cmd(300);
    format!(
        "command: {cmd}\nargs:\n{}condition_config_any:\n  - path: '{agent_yaml}'\n    keys:\n      - process_config.process_collection.enabled\nauto_start: true\n",
        args.iter()
            .map(|arg| format!("  - '{arg}'\n"))
            .collect::<String>()
    )
}

fn gated_env(process_collection_enabled: bool) -> TestEnv {
    let env = TestEnv::new();
    let agent_yaml = env.env_root().join("datadog.yaml");
    write_agent_yaml(&agent_yaml, process_collection_enabled);
    env.with_config(
        "gated-sleeper",
        &gated_sleeper_yaml(&agent_yaml.to_string_lossy()),
    )
}

#[test]
fn config_gate_open_auto_starts_process() {
    let procmgr = gated_env(true).start();
    procmgr
        .wait_for_process_running("gated-sleeper")
        .expect("expected gated-sleeper running when gate is open");
    procmgr
        .require_list()
        .assert_process_state("gated-sleeper", ProcessExpect::Running);
}

#[test]
fn config_gate_closed_stays_created() {
    let procmgr = gated_env(false).start();
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("gated-sleeper", ProcessExpect::Created);
    procmgr.assert_daemon_log_line_contains(&["gated-sleeper", "condition_config_any not met"]);
}

#[test]
fn reload_restarts_created_process_when_gate_opens() {
    let env = gated_env(false);
    let agent_yaml = env.env_root().join("datadog.yaml");
    let procmgr = env.start();

    procmgr
        .require_list()
        .assert_process_state("gated-sleeper", ProcessExpect::Created);

    write_agent_yaml(&agent_yaml, true);
    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["gated-sleeper".into()]),
        ..Default::default()
    });

    procmgr
        .wait_for_process_running("gated-sleeper")
        .expect("expected gated-sleeper running after gate opens on reload");
}
