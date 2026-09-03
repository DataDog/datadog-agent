// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{DescribeExpect, ProcessExpect, TestEnv};
use dd_procmgrd::test_helpers;

const PROCESS_AGENT_NAME: &str = "datadog-agent-process";

fn expected_process_agent_profile() -> &'static str {
    if cfg!(windows) { "privileged" } else { "agent" }
}

fn mixed_catalog_env() -> TestEnv {
    TestEnv::new()
        .with_process("sleeper")
        .with_process(PROCESS_AGENT_NAME)
}

#[test]
fn mixed_catalog_agent_and_process_name_profiles() {
    let procmgr = mixed_catalog_env().start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let list = procmgr.require_list();
    list.assert_len(2);
    list.assert_process_state("sleeper", ProcessExpect::Running);
    list.assert_process_state(PROCESS_AGENT_NAME, ProcessExpect::Created);

    assert_eq!(list.require_process("sleeper").profile, "agent");
    assert_eq!(
        list.require_process(PROCESS_AGENT_NAME).profile,
        expected_process_agent_profile(),
    );

    for name in ["sleeper", PROCESS_AGENT_NAME] {
        assert_eq!(
            list.require_process(name).user,
            test_helpers::expected_spawn_user(name),
            "list user for {name}"
        );
    }
}

#[test]
fn describe_datadog_agent_process_profile_matches_platform() {
    let procmgr = mixed_catalog_env().start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let list_snap = procmgr
        .process(PROCESS_AGENT_NAME)
        .expect("datadog-agent-process in catalog");

    procmgr.assert_describe_matches(
        PROCESS_AGENT_NAME,
        DescribeExpect {
            name: Some(PROCESS_AGENT_NAME.into()),
            state: Some("Created".into()),
            profile: Some(expected_process_agent_profile().into()),
            user: Some(test_helpers::expected_spawn_user(PROCESS_AGENT_NAME)),
            command: Some(list_snap.command),
            args: Some(list_snap.args),
            has_uuid: Some(true),
            pid_alive: Some(false),
            ..Default::default()
        },
    );
}

#[test]
fn list_json_exposes_profile_for_both() {
    let procmgr = mixed_catalog_env().start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");

    let out = procmgr.cli_list_json();
    out.assert_success();
    let json = out.stdout_json();
    let arr = json.as_array().expect("expected JSON array");
    assert_eq!(arr.len(), 2);

    for name in ["sleeper", PROCESS_AGENT_NAME] {
        let entry = arr
            .iter()
            .find(|entry| entry["name"] == name)
            .unwrap_or_else(|| panic!("list JSON missing process {name}: {json}"));
        let expected_profile = if name == PROCESS_AGENT_NAME {
            expected_process_agent_profile()
        } else {
            "agent"
        };
        assert_eq!(entry["profile"], expected_profile, "profile for {name}");
        assert_eq!(
            entry["user"],
            test_helpers::expected_spawn_user(name),
            "user for {name}"
        );
    }
}
