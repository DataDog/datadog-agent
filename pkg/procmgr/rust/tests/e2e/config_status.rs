// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::TestEnv;
use dd_procmgrd::test_helpers;

#[test]
fn test_cli_config_basic() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    let config_dir = env.config_dir().display().to_string();

    env.cli_config()
        .assert_success()
        .assert_field("Source", "yaml")
        .assert_field("Location", &config_dir)
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "0");
}

#[test]
fn test_cli_config_json() {
    let env = TestEnv::new()
        .with_config("svc-a", test_helpers::sleep_config_yaml())
        .with_config("svc-b", test_helpers::sleep_config_yaml())
        .start();

    let config_dir = env.config_dir().display().to_string();

    let out = env.cli_config_json();
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["source"], "yaml");
    assert_eq!(json["location"], config_dir.as_str());
    assert_eq!(json["loaded_processes"], 2);
    assert_eq!(json["runtime_processes"], 0);
}

#[test]
fn test_cli_config_with_runtime_processes() {
    let env = TestEnv::new()
        .with_config("loaded", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[loaded] spawned");

    env.cli_config()
        .assert_success()
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "0");

    env.create_sleep("dynamic", &[]).assert_success();

    env.cli_config()
        .assert_success()
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "1");
}

#[test]
fn test_cli_status_basic() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli_status()
        .assert_success()
        .assert_field("Ready", "true")
        .assert_has_field("Version")
        .assert_has_field("Uptime")
        .assert_field("Total Processes", "1")
        .assert_field("Running", "1")
        .assert_field("Stopped", "0")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");
}

#[test]
fn test_cli_status_counts() {
    let env = TestEnv::new()
        .with_config("runner-a", test_helpers::sleep_config_yaml())
        .with_config("runner-b", test_helpers::sleep_config_yaml())
        .with_config(
            "idle",
            &test_helpers::sleep_config_with("auto_start: false\n"),
        )
        .start();

    env.daemon().wait_for_log_default("[runner-a] spawned");
    env.daemon().wait_for_log_default("[runner-b] spawned");

    env.cli_status()
        .assert_success()
        .assert_field("Total Processes", "3")
        .assert_field("Running", "2")
        .assert_field("Created", "1")
        .assert_field("Stopped", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");
}

#[test]
fn test_cli_status_json() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .with_config(
            "idle",
            &test_helpers::sleep_config_with("auto_start: false\n"),
        )
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli_status_json();
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["ready"], true);
    assert!(!json["version"].as_str().unwrap_or("").is_empty());
    assert!(json["uptime_seconds"].as_u64().is_some());
    assert_eq!(json["total_processes"], 2);
    assert_eq!(json["running_processes"], 1);
    assert_eq!(json["created_processes"], 1);
    assert_eq!(json["stopped_processes"], 0);
    assert_eq!(json["failed_processes"], 0);
    assert_eq!(json["exited_processes"], 0);
    assert_eq!(json["starting_processes"], 0);
    assert_eq!(json["stopping_processes"], 0);
}

#[test]
fn test_cli_status_after_stop() {
    let env = TestEnv::new()
        .with_config("svc-a", test_helpers::sleep_config_yaml())
        .with_config("svc-b", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[svc-a] spawned");
    env.daemon().wait_for_log_default("[svc-b] spawned");

    env.cli_status()
        .assert_success()
        .assert_field("Running", "2")
        .assert_field("Stopped", "0");

    env.cli_stop("svc-a").assert_success();

    env.cli_status()
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "1")
        .assert_field("Stopped", "1");

    let json = env.cli_status_json().stdout_json();
    assert_eq!(json["running_processes"], 1);
    assert_eq!(json["stopped_processes"], 1);
}

#[test]
fn test_cli_status_mixed_states() {
    let env = TestEnv::new()
        .with_config("runner", test_helpers::sleep_config_yaml())
        .with_config("bad", "command: /nonexistent/binary\n")
        .with_config("quick", test_helpers::true_config_yaml())
        .with_config(
            "idle",
            &test_helpers::sleep_config_with("auto_start: false\n"),
        )
        .start();

    env.daemon().wait_for_log_default("[runner] spawned");
    env.daemon().wait_for_log_default("[bad] failed to spawn");
    env.daemon().wait_for_log_default("[quick] exited with");

    let out = env.cli_status();
    out.assert_success()
        .assert_field("Total Processes", "4")
        .assert_field("Running", "1")
        .assert_field("Created", "1")
        .assert_field("Failed", "1")
        .assert_field("Exited", "1")
        .assert_field("Stopped", "0");

    let json_out = env.cli_status_json();
    json_out.assert_success();
    let json = json_out.stdout_json();
    assert_eq!(json["total_processes"], 4);
    assert_eq!(json["running_processes"], 1);
    assert_eq!(json["created_processes"], 1);
    assert_eq!(json["failed_processes"], 1);
    assert_eq!(json["exited_processes"], 1);
    assert_eq!(json["stopped_processes"], 0);
}

#[test]
fn test_cli_status_reflects_operations() {
    let env = TestEnv::new()
        .with_config("svc-a", test_helpers::sleep_config_yaml())
        .with_config("svc-b", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[svc-a] spawned");
    env.daemon().wait_for_log_default("[svc-b] spawned");

    env.cli_status()
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "2")
        .assert_field("Stopped", "0")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");

    env.cli_stop("svc-a").assert_success();

    env.cli_status()
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "1")
        .assert_field("Stopped", "1")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");

    env.create_sleep("svc-c", &[]).assert_success();
    env.daemon().wait_for_log_default("[svc-c] spawned");

    env.cli_status()
        .assert_success()
        .assert_field("Total Processes", "3")
        .assert_field("Running", "2")
        .assert_field("Stopped", "1")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");
}
