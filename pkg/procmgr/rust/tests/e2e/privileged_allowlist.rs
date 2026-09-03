// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![cfg(windows)]

use crate::helpers::{ProcessExpect, TestEnv};
use dd_procmgrd::test_helpers;

const PROCESS_AGENT_NAME: &str = "datadog-agent-process";

fn env_with_privileged_yaml(yaml: &str) -> TestEnv {
    TestEnv::new().with_config(PROCESS_AGENT_NAME, yaml)
}

#[test]
fn privileged_spawn_fails_when_supervisor_not_local_system() {
    let procmgr =
        env_with_privileged_yaml(&test_helpers::privileged_process_agent_yaml("")).start();

    procmgr.assert_process_state_within(PROCESS_AGENT_NAME, ProcessExpect::Failed);
    assert_eq!(
        procmgr
            .require_list()
            .require_process(PROCESS_AGENT_NAME)
            .profile,
        "privileged"
    );
    procmgr.assert_daemon_log_line_contains(&[
        PROCESS_AGENT_NAME,
        "privileged spawn requires dd-procmgrd to run as LocalSystem",
    ]);
}

#[test]
fn privileged_spawn_refuses_wrong_command() {
    let (_cmd, args) = test_helpers::privileged_process_agent_command_line();
    let yaml = test_helpers::cmd_yaml(r"C:\wrong\process-agent.exe", &args, "auto_start: true\n");
    let procmgr = env_with_privileged_yaml(&yaml).start();

    procmgr.assert_process_state_within(PROCESS_AGENT_NAME, ProcessExpect::Failed);
    procmgr.assert_daemon_log_line_contains(&[
        PROCESS_AGENT_NAME,
        "refusing privileged spawn: unexpected command",
    ]);
}

#[test]
fn privileged_spawn_refuses_wrong_cfgpath() {
    let (cmd, mut args) = test_helpers::privileged_process_agent_command_line();
    args[1] = r"C:\wrong\datadog.yaml".to_string();
    let yaml = test_helpers::cmd_yaml(
        &format!("'{}'", cmd.replace('\'', "''")),
        &args,
        "auto_start: true\n",
    );
    let procmgr = env_with_privileged_yaml(&yaml).start();

    procmgr.assert_process_state_within(PROCESS_AGENT_NAME, ProcessExpect::Failed);
    procmgr.assert_daemon_log_line_contains(&[
        PROCESS_AGENT_NAME,
        "refusing privileged spawn: unexpected args",
    ]);
}

#[test]
fn privileged_spawn_refuses_yaml_env() {
    let yaml = test_helpers::privileged_process_agent_yaml("env:\n  DD_FOO: bar\n");
    let procmgr = env_with_privileged_yaml(&yaml).start();

    procmgr.assert_process_state_within(PROCESS_AGENT_NAME, ProcessExpect::Failed);
    procmgr.assert_daemon_log_line_contains(&[
        PROCESS_AGENT_NAME,
        "refusing privileged spawn: disallowed env vars",
    ]);
}

#[test]
fn privileged_spawn_refuses_working_dir() {
    let working_dir = test_helpers::temp_dir_str();
    let yaml =
        test_helpers::privileged_process_agent_yaml(&format!("working_dir: '{working_dir}'\n"));
    let procmgr = env_with_privileged_yaml(&yaml).start();

    procmgr.assert_process_state_within(PROCESS_AGENT_NAME, ProcessExpect::Failed);
    procmgr.assert_daemon_log_line_contains(&[
        PROCESS_AGENT_NAME,
        "refusing privileged spawn: working_dir is not allowed",
    ]);
}
