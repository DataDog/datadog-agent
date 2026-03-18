// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod helpers;

use helpers::{CliRunner, TestEnv, pid_is_alive, wait_for_pid_gone, write_config};
use std::path::Path;
use std::time::Duration;

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

#[test]
fn test_cli_config_json() {
    let env = TestEnv::new()
        .with_config("svc-a", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("svc-b", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    let config_dir = env.config_dir().display().to_string();

    let out = env.cli(&["config", "--json"]);
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["source"], "yaml");
    assert_eq!(json["location"], config_dir.as_str());
    assert_eq!(json["loaded_processes"], 2);
    assert_eq!(json["runtime_processes"], 0);
}

#[test]
fn test_cli_status_basic() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["status"])
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
        .with_config("runner-a", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("runner-b", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config(
            "idle",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.daemon().wait_for_log_default("[runner-a] spawned");
    env.daemon().wait_for_log_default("[runner-b] spawned");

    env.cli(&["status"])
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
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config(
            "idle",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["status", "--json"]);
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
        .with_config("svc-a", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("svc-b", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[svc-a] spawned");
    env.daemon().wait_for_log_default("[svc-b] spawned");

    env.cli(&["status"])
        .assert_success()
        .assert_field("Running", "2")
        .assert_field("Stopped", "0");

    env.cli(&["stop", "svc-a"]).assert_success();

    env.cli(&["status"])
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "1")
        .assert_field("Stopped", "1");

    let json = env.cli(&["status", "--json"]).stdout_json();
    assert_eq!(json["running_processes"], 1);
    assert_eq!(json["stopped_processes"], 1);
}

#[test]
fn test_cli_status_mixed_states() {
    let env = TestEnv::new()
        .with_config("runner", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("bad", "command: /nonexistent/binary\n")
        .with_config("quick", "command: /usr/bin/true\nrestart: never\n")
        .with_config(
            "idle",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.daemon().wait_for_log_default("[runner] spawned");
    env.daemon().wait_for_log_default("[bad] failed to spawn");
    env.daemon().wait_for_log_default("[quick] exited with");

    let out = env.cli(&["status"]);
    out.assert_success()
        .assert_field("Total Processes", "4")
        .assert_field("Running", "1")
        .assert_field("Created", "1")
        .assert_field("Failed", "1")
        .assert_field("Exited", "1")
        .assert_field("Stopped", "0");

    let json_out = env.cli(&["status", "--json"]);
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
fn test_cli_config_with_runtime_processes() {
    let env = TestEnv::new()
        .with_config("loaded", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[loaded] spawned");

    env.cli(&["config"])
        .assert_success()
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "0");

    env.cli(&[
        "create",
        "--name",
        "dynamic",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_success();

    env.cli(&["config"])
        .assert_success()
        .assert_field("Loaded Processes", "1")
        .assert_field("Runtime Processes", "1");
}

#[test]
fn test_cli_list_empty() {
    let env = TestEnv::new().start();

    env.cli(&["list"])
        .assert_success()
        .assert_stdout_contains("No processes");
}

#[test]
fn test_cli_list_one_running() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row(
            "sleeper",
            &[("STATE", "Running"), ("COMMAND", "/bin/sleep")],
        )
        .assert_table_row_count(1);

    let pid = out.pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_list_multiple_processes() {
    let env = TestEnv::new()
        .with_config("alpha", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("beta", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[alpha] spawned");
    env.daemon().wait_for_log_default("[beta] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row("alpha", &[("STATE", "Running")])
        .assert_table_row("beta", &[("STATE", "Running")])
        .assert_table_row_count(2);

    let pid_a = out.pid_from_table_row("alpha");
    let pid_b = out.pid_from_table_row("beta");
    assert!(pid_is_alive(pid_a), "alpha PID {pid_a} should be alive");
    assert!(pid_is_alive(pid_b), "beta PID {pid_b} should be alive");
}

#[test]
fn test_cli_list_mixed_states() {
    let env = TestEnv::new()
        .with_config("runner", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config(
            "idle",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.daemon().wait_for_log_default("[runner] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row("runner", &[("STATE", "Running")])
        .assert_table_row("idle", &[("STATE", "Created"), ("PID", "-")])
        .assert_table_row_count(2);

    let pid = out.pid_from_table_row("runner");
    assert!(pid_is_alive(pid), "runner PID {pid} should be alive");
}

#[test]
fn test_cli_list_spawn_failure() {
    let env = TestEnv::new()
        .with_config("bad", "command: /nonexistent/binary\n")
        .start();

    env.daemon().wait_for_log_default("[bad] failed to spawn");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("bad", &[("STATE", "Failed"), ("PID", "-")])
        .assert_table_row_count(1);
}

#[test]
fn test_cli_list_exited_state() {
    let env = TestEnv::new()
        .with_config("quick", "command: /usr/bin/true\nrestart: never\n")
        .start();

    env.daemon().wait_for_log_default("[quick] exited with");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row(
            "quick",
            &[("STATE", "Exited"), ("PID", "-"), ("LAST EXIT", "exit 0")],
        )
        .assert_table_row_count(1);
}

#[test]
fn test_cli_list_last_exit_column() {
    let env = TestEnv::new()
        .with_config("ok", "command: /usr/bin/true\nrestart: never\n")
        .with_config("fail", "command: /usr/bin/false\nrestart: never\n")
        .with_config("alive", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[ok] exited with");
    env.daemon().wait_for_log_default("[fail] exited with");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("ok", &[("LAST EXIT", "exit 0")])
        .assert_table_row("fail", &[("LAST EXIT", "exit 1")])
        .assert_table_row("alive", &[("LAST EXIT", "-")]);
}

#[test]
fn test_cli_list_json() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["list", "--json"]);
    out.assert_success();
    let json = out.stdout_json();
    let arr = json.as_array().expect("expected JSON array");
    assert_eq!(arr.len(), 1);

    let entry = &arr[0];
    assert_eq!(entry["name"], "sleeper");
    assert_eq!(entry["state"], "Running");
    assert_eq!(entry["command"], "/bin/sleep");
    assert_eq!(entry["args"], serde_json::json!(["300"]));
    assert_eq!(entry["restart_count"], 0);
    assert!(!entry["uuid"].as_str().unwrap_or("").is_empty());
    assert!(entry["last_exit_code"].is_null());
    assert!(entry["last_signal"].is_null());

    let pid = entry["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0, "running process should have a PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_list_json_empty() {
    let env = TestEnv::new().start();

    let out = env.cli(&["list", "--json"]);
    out.assert_success();
    let json = out.stdout_json();
    let arr = json.as_array().expect("expected JSON array");
    assert!(arr.is_empty(), "expected empty array, got {json}");
}

#[test]
fn test_cli_list_shows_restart_count() {
    let env = TestEnv::new()
        .with_config("crasher", "command: /usr/bin/false\nrestart: always\n")
        .start();

    assert!(
        env.daemon()
            .wait_for_log_count("[crasher] spawned", 3, Duration::from_secs(10)),
        "crasher should have restarted at least twice"
    );

    env.cli(&["list"])
        .assert_success()
        .assert_table_row_count(1);

    let out = env.cli(&["list", "--json"]);
    out.assert_success();
    let json = out.stdout_json();
    let count = json[0]["restart_count"]
        .as_u64()
        .expect("restart_count should be a number");
    assert!(count >= 2, "expected restart_count >= 2, got {count}");
}

#[test]
fn test_cli_describe_by_name() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["describe", "sleeper"]);
    out.assert_success()
        .assert_field("Name", "sleeper")
        .assert_field("State", "Running")
        .assert_field("Command", "/bin/sleep")
        .assert_field("Args", "300")
        .assert_has_field("UUID");

    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_describe_by_uuid() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let list_out = env.cli(&["list", "--json"]);
    let json = list_out.stdout_json();
    let uuid = json[0]["uuid"].as_str().expect("uuid should be a string");
    let prefix = &uuid[..8];

    let out = env.cli(&["describe", prefix]);
    out.assert_success()
        .assert_field("Name", "sleeper")
        .assert_field("UUID", uuid);

    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");

    env.cli(&["describe", uuid])
        .assert_success()
        .assert_field("Name", "sleeper")
        .assert_field("UUID", uuid);
}

#[test]
fn test_cli_describe_shows_all_fields() {
    let env = TestEnv::new()
        .with_config(
            "full",
            concat!(
                "command: /bin/sleep\n",
                "args:\n  - '300'\n",
                "description: a test process\n",
                "working_dir: /tmp\n",
                "env:\n  MY_VAR: hello\n",
                "restart: always\n",
                "after:\n  - other\n",
            ),
        )
        .start();

    env.daemon().wait_for_log_default("[full] spawned");

    let out = env.cli(&["describe", "full"]);
    out.assert_success()
        .assert_field("Name", "full")
        .assert_field("State", "Running")
        .assert_field("Command", "/bin/sleep")
        .assert_field("Args", "300")
        .assert_field("Description", "a test process")
        .assert_field("Working Dir", "/tmp")
        .assert_field("Restart Policy", "always")
        .assert_field("Auto Start", "true")
        .assert_has_field("UUID")
        .assert_has_field("Stdout")
        .assert_has_field("Stderr");

    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_describe_after_exit() {
    let env = TestEnv::new()
        .with_config("quick", "command: /usr/bin/false\nrestart: never\n")
        .start();

    env.daemon().wait_for_log_default("[quick] exited with");

    env.cli(&["describe", "quick"])
        .assert_success()
        .assert_field("Name", "quick")
        .assert_field("State", "Failed")
        .assert_field("PID", "-")
        .assert_field("Last Exit", "exit 1");
}

#[test]
fn test_cli_describe_after_restart() {
    let env = TestEnv::new()
        .with_config("crasher", "command: /usr/bin/false\nrestart: always\n")
        .start();

    assert!(
        env.daemon()
            .wait_for_log_count("[crasher] spawned", 3, Duration::from_secs(10)),
        "crasher should have restarted at least twice"
    );

    let out = env.cli(&["describe", "crasher"]);
    out.assert_success().assert_field("Name", "crasher");

    let restarts: u32 = out.field_value("Restarts").parse().unwrap();
    assert!(restarts >= 2, "expected Restarts >= 2, got {restarts}");
}

#[test]
fn test_cli_describe_not_found() {
    let env = TestEnv::new().start();

    env.cli(&["describe", "nonexistent"])
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn test_cli_describe_json() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["describe", "--json", "sleeper"]);
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["name"], "sleeper");
    assert_eq!(json["state"], "Running");
    assert_eq!(json["command"], "/bin/sleep");
    assert_eq!(json["args"], serde_json::json!(["300"]));
    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());

    let pid = json["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0);
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_start_stopped_process() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("sleeper", &[("STATE", "Created")]);

    env.cli(&["start", "sleeper"])
        .assert_success()
        .assert_field("State", "Running");

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["describe", "sleeper"]);
    out.assert_success().assert_field("State", "Running");

    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_start_by_uuid() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    let list_json = env.cli(&["list", "--json"]).stdout_json();
    let uuid = list_json[0]["uuid"]
        .as_str()
        .expect("uuid should be a string");
    let prefix = &uuid[..8];

    env.cli(&["start", prefix])
        .assert_success()
        .assert_field("State", "Running");

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli(&["describe", "sleeper"]).pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_start_already_running() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["start", "sleeper"])
        .assert_failure()
        .assert_stderr_contains("already");
}

#[test]
fn test_cli_start_not_found() {
    let env = TestEnv::new().start();

    env.cli(&["start", "nonexistent"])
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn test_cli_start_json() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    let out = env.cli(&["start", "--json", "sleeper"]);
    out.assert_success();
    let json = out.stdout_json();

    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
    assert_eq!(json["state"], "Running");

    let pid = json["pid"].as_u64().expect("pid should be a number") as u32;
    assert!(pid > 0, "started process should have a PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_start_then_verify_list() {
    let env = TestEnv::new()
        .with_config(
            "sleeper",
            "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
        )
        .start();

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("sleeper", &[("STATE", "Created"), ("PID", "-")]);

    env.cli(&["start", "sleeper"]).assert_success();
    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row("sleeper", &[("STATE", "Running")]);

    let pid = out.pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_stop_running_process() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["stop", "sleeper"])
        .assert_success()
        .assert_field("State", "Stopped");
}

#[test]
fn test_cli_stop_by_uuid() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let list_json = env.cli(&["list", "--json"]).stdout_json();
    let uuid = list_json[0]["uuid"]
        .as_str()
        .expect("uuid should be a string");
    let prefix = &uuid[..8];

    env.cli(&["stop", prefix])
        .assert_success()
        .assert_field("State", "Stopped");
}

#[test]
fn test_cli_stop_already_stopped() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["stop", "sleeper"]).assert_success();
    env.daemon().wait_for_log_default("[sleeper] stopped");

    env.cli(&["stop", "sleeper"])
        .assert_failure()
        .assert_stderr_contains("not running");
}

#[test]
fn test_cli_stop_not_found() {
    let env = TestEnv::new().start();

    env.cli(&["stop", "nonexistent"])
        .assert_failure()
        .assert_stderr_contains("not found");
}

#[test]
fn test_cli_stop_json() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let out = env.cli(&["stop", "--json", "sleeper"]);
    out.assert_success();
    let json = out.stdout_json();

    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
    assert_eq!(json["state"], "Stopped");
}

#[test]
fn test_cli_stop_then_verify_list() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["stop", "sleeper"]).assert_success();
    env.daemon().wait_for_log_default("[sleeper] stopped");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("sleeper", &[("STATE", "Stopped"), ("PID", "-")]);
}

#[test]
fn test_cli_stop_kills_child() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli(&["list"]).pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "PID {pid} should be alive before stop");

    env.cli(&["stop", "sleeper"]).assert_success();

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "child PID {pid} should be gone after stop"
    );
}

#[test]
fn test_cli_create_minimal() {
    let env = TestEnv::new().start();

    let out = env.cli(&[
        "create",
        "--name",
        "foo",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ]);
    out.assert_success().assert_has_field("UUID");

    env.daemon().wait_for_log_default("[foo] spawned");

    let pid = env.cli(&["describe", "foo"]).pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_create_with_auto_start() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_success();

    env.daemon().wait_for_log_default("[svc] spawned");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("svc", &[("STATE", "Running")]);

    let pid = env.cli(&["list"]).pid_from_table_row("svc");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_create_no_auto_start() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "manual",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--no-auto-start",
    ])
    .assert_success();

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("manual", &[("STATE", "Created"), ("PID", "-")]);
}

#[test]
fn test_cli_create_with_all_options() {
    let env = TestEnv::new()
        .with_config("dep", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[dep] spawned");

    env.cli(&[
        "create",
        "--name",
        "full",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--env",
        "KEY1=val1",
        "--env",
        "KEY2=val2",
        "--working-dir",
        "/tmp",
        "--restart-policy",
        "always",
        "--description",
        "full test",
        "--after",
        "dep",
    ])
    .assert_success();

    env.daemon().wait_for_log_default("[full] spawned");

    let out = env.cli(&["describe", "full"]);
    out.assert_success()
        .assert_field("Name", "full")
        .assert_field("Command", "/bin/sleep")
        .assert_field("Working Dir", "/tmp")
        .assert_field("Restart Policy", "always")
        .assert_field("Description", "full test");
}

#[test]
fn test_cli_create_then_describe() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--no-auto-start",
    ])
    .assert_success();

    let out = env.cli(&["describe", "svc"]);
    out.assert_success()
        .assert_field("Name", "svc")
        .assert_field("State", "Created")
        .assert_field("Command", "/bin/sleep")
        .assert_field("Args", "300")
        .assert_field("PID", "-")
        .assert_has_field("UUID");
}

#[test]
fn test_cli_create_duplicate_name() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "dup",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_success();

    env.daemon().wait_for_log_default("[dup] spawned");

    env.cli(&[
        "create",
        "--name",
        "dup",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_failure()
    .assert_stderr_contains("already exists");
}

#[test]
fn test_cli_create_empty_command() {
    let env = TestEnv::new().start();

    env.cli(&["create", "--name", "foo", "--command", ""])
        .assert_failure()
        .assert_stderr_contains("command must not be empty");
}

#[test]
fn test_cli_create_invalid_name() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "bad name!",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_failure()
    .assert_stderr_contains("name must only contain");
}

#[test]
fn test_cli_create_json() {
    let env = TestEnv::new().start();

    let out = env.cli(&[
        "create",
        "--json",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ]);
    out.assert_success();
    let json = out.stdout_json();

    assert_eq!(json["name"], "svc");
    assert!(!json["uuid"].as_str().unwrap_or("").is_empty());
}

#[test]
fn test_cli_create_env_vars() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "env-svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--env",
        "FOO=bar",
        "--env",
        "BAZ=qux",
        "--no-auto-start",
    ])
    .assert_success();

    let out = env.cli(&["describe", "--json", "env-svc"]);
    out.assert_success();
    let json = out.stdout_json();

    let env_map = json["env"].as_object().expect("env should be an object");
    assert_eq!(env_map.get("FOO").and_then(|v| v.as_str()), Some("bar"));
    assert_eq!(env_map.get("BAZ").and_then(|v| v.as_str()), Some("qux"));
}

#[test]
fn test_cli_reload_no_changes() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    env.cli(&["reload"])
        .assert_success()
        .assert_field("Unchanged", "sleeper");
}

#[test]
fn test_cli_reload_add_process() {
    let env = TestEnv::new()
        .with_config("existing", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[existing] spawned");

    write_config(
        env.config_dir(),
        "new-svc",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    env.cli(&["reload"])
        .assert_success()
        .assert_field("Added", "new-svc");

    env.daemon().wait_for_log_default("[new-svc] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row("new-svc", &[("STATE", "Running")])
        .assert_table_row("existing", &[("STATE", "Running")])
        .assert_table_row_count(2);

    let pid_new = out.pid_from_table_row("new-svc");
    let pid_existing = out.pid_from_table_row("existing");
    assert!(
        pid_is_alive(pid_new),
        "new-svc PID {pid_new} should be alive"
    );
    assert!(
        pid_is_alive(pid_existing),
        "existing PID {pid_existing} should be alive"
    );
}

#[test]
fn test_cli_reload_remove_process() {
    let env = TestEnv::new()
        .with_config("keeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("doomed", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[keeper] spawned");
    env.daemon().wait_for_log_default("[doomed] spawned");

    let doomed_pid = env.cli(&["list"]).pid_from_table_row("doomed");

    std::fs::remove_file(env.config_dir().join("doomed.yaml"))
        .expect("failed to remove doomed.yaml");

    env.cli(&["reload"])
        .assert_success()
        .assert_field("Removed", "doomed");

    assert!(
        wait_for_pid_gone(doomed_pid, Duration::from_secs(5)),
        "removed process PID {doomed_pid} should be gone"
    );

    env.cli(&["list"])
        .assert_success()
        .assert_table_row_count(1)
        .assert_table_row("keeper", &[("STATE", "Running")]);
}

#[test]
fn test_cli_reload_modify_process() {
    let env = TestEnv::new()
        .with_config("svc", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[svc] spawned");

    let old_pid = env.cli(&["list"]).pid_from_table_row("svc");

    write_config(
        env.config_dir(),
        "svc",
        "command: /bin/sleep\nargs:\n  - '600'\n",
    );

    env.cli(&["reload"])
        .assert_success()
        .assert_field("Modified", "svc");

    assert!(
        env.daemon()
            .wait_for_log_count("[svc] spawned", 2, Duration::from_secs(10)),
        "svc should have been respawned after modify"
    );

    assert!(
        wait_for_pid_gone(old_pid, Duration::from_secs(5)),
        "old PID {old_pid} should be gone after modify+reload"
    );

    let new_pid = env.cli(&["list"]).pid_from_table_row("svc");
    assert!(pid_is_alive(new_pid), "new PID {new_pid} should be alive");
    assert_ne!(old_pid, new_pid, "PID should change after modify+reload");
}

#[test]
fn test_cli_reload_add_and_remove() {
    let env = TestEnv::new()
        .with_config("old", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[old] spawned");

    let old_pid = env.cli(&["list"]).pid_from_table_row("old");

    std::fs::remove_file(env.config_dir().join("old.yaml")).expect("failed to remove old.yaml");
    write_config(
        env.config_dir(),
        "new",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let out = env.cli(&["reload"]);
    out.assert_success()
        .assert_field("Added", "new")
        .assert_field("Removed", "old");

    assert!(
        wait_for_pid_gone(old_pid, Duration::from_secs(5)),
        "old PID {old_pid} should be gone after removal"
    );

    env.daemon().wait_for_log_default("[new] spawned");

    let new_pid = env.cli(&["list"]).pid_from_table_row("new");
    assert!(pid_is_alive(new_pid), "new PID {new_pid} should be alive");
}

#[test]
fn test_cli_reload_json() {
    let env = TestEnv::new()
        .with_config("existing", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[existing] spawned");

    write_config(
        env.config_dir(),
        "added",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let out = env.cli(&["reload", "--json"]);
    out.assert_success();
    let json = out.stdout_json();

    let added = json["added"].as_array().expect("added should be an array");
    assert!(
        added.iter().any(|v| v.as_str() == Some("added")),
        "expected 'added' in added array: {json}"
    );

    let unchanged = json["unchanged"]
        .as_array()
        .expect("unchanged should be an array");
    assert!(
        unchanged.iter().any(|v| v.as_str() == Some("existing")),
        "expected 'existing' in unchanged array: {json}"
    );

    assert!(json["removed"].as_array().expect("array").is_empty());
    assert!(json["modified"].as_array().expect("array").is_empty());
}

#[test]
fn test_cli_reload_new_process_starts() {
    let env = TestEnv::new().start();

    write_config(
        env.config_dir(),
        "late",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    env.cli(&["reload"]).assert_success();
    env.daemon().wait_for_log_default("[late] spawned");

    let out = env.cli(&["list"]);
    out.assert_success()
        .assert_table_row("late", &[("STATE", "Running")]);

    let pid = out.pid_from_table_row("late");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_reload_removed_process_stopped() {
    let env = TestEnv::new()
        .with_config("ephemeral", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[ephemeral] spawned");

    let pid = env.cli(&["list"]).pid_from_table_row("ephemeral");

    std::fs::remove_file(env.config_dir().join("ephemeral.yaml"))
        .expect("failed to remove ephemeral.yaml");

    env.cli(&["reload"]).assert_success();

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "removed process PID {pid} should be gone"
    );

    env.cli(&["list"])
        .assert_success()
        .assert_stdout_contains("No processes");
}

#[test]
fn test_cli_full_lifecycle() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--no-auto-start",
    ])
    .assert_success();

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("svc", &[("STATE", "Created"), ("PID", "-")]);

    env.cli(&["start", "svc"]).assert_success();
    env.daemon().wait_for_log_default("[svc] spawned");

    let out = env.cli(&["describe", "svc"]);
    out.assert_success().assert_field("State", "Running");
    let pid = out.pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");

    env.cli(&["stop", "svc"]).assert_success();

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "PID {pid} should be gone after stop"
    );

    env.cli(&["describe", "svc"])
        .assert_success()
        .assert_field("State", "Stopped")
        .assert_field("PID", "-");
}

#[test]
fn test_cli_create_stop_start_cycle() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_success();
    env.daemon().wait_for_log_default("[svc] spawned");

    env.cli(&["describe", "svc"])
        .assert_success()
        .assert_field("State", "Running");

    env.cli(&["stop", "svc"]).assert_success();
    env.cli(&["describe", "svc"])
        .assert_success()
        .assert_field("State", "Stopped");

    env.cli(&["start", "svc"]).assert_success();
    env.daemon()
        .wait_for_log_count("[svc] spawned", 2, Duration::from_secs(10));
    env.cli(&["describe", "svc"])
        .assert_success()
        .assert_field("State", "Running");

    let pid = env.cli(&["describe", "svc"]).pid_from_field("PID");
    assert!(pid_is_alive(pid), "PID {pid} should be alive after restart");

    env.cli(&["stop", "svc"]).assert_success();
    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "PID {pid} should be gone after second stop"
    );

    env.cli(&["start", "svc"]).assert_success();
    env.daemon()
        .wait_for_log_count("[svc] spawned", 3, Duration::from_secs(10));

    let new_pid = env.cli(&["describe", "svc"]).pid_from_field("PID");
    assert!(pid_is_alive(new_pid), "PID {new_pid} should be alive");
}

#[test]
fn test_cli_reload_then_start() {
    let env = TestEnv::new().start();

    write_config(
        env.config_dir(),
        "late",
        "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
    );

    env.cli(&["reload"])
        .assert_success()
        .assert_field("Added", "late");

    env.cli(&["list"])
        .assert_success()
        .assert_table_row("late", &[("STATE", "Created"), ("PID", "-")]);

    env.cli(&["start", "late"]).assert_success();
    env.daemon().wait_for_log_default("[late] spawned");

    let pid = env.cli(&["list"]).pid_from_table_row("late");
    assert!(pid_is_alive(pid), "PID {pid} should be alive");
}

#[test]
fn test_cli_status_reflects_operations() {
    let env = TestEnv::new()
        .with_config("svc-a", "command: /bin/sleep\nargs:\n  - '300'\n")
        .with_config("svc-b", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[svc-a] spawned");
    env.daemon().wait_for_log_default("[svc-b] spawned");

    env.cli(&["status"])
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "2")
        .assert_field("Stopped", "0")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");

    env.cli(&["stop", "svc-a"]).assert_success();

    env.cli(&["status"])
        .assert_success()
        .assert_field("Total Processes", "2")
        .assert_field("Running", "1")
        .assert_field("Stopped", "1")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");

    env.cli(&[
        "create",
        "--name",
        "svc-c",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
    ])
    .assert_success();
    env.daemon().wait_for_log_default("[svc-c] spawned");

    env.cli(&["status"])
        .assert_success()
        .assert_field("Total Processes", "3")
        .assert_field("Running", "2")
        .assert_field("Stopped", "1")
        .assert_field("Created", "0")
        .assert_field("Failed", "0")
        .assert_field("Exited", "0");
}

#[test]
fn test_cli_create_with_dependencies() {
    let env = TestEnv::new().start();

    env.cli(&[
        "create",
        "--name",
        "backend",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--no-auto-start",
    ])
    .assert_success();

    env.cli(&[
        "create",
        "--name",
        "frontend",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--after",
        "backend",
        "--no-auto-start",
    ])
    .assert_success();

    env.cli(&["list"])
        .assert_success()
        .assert_table_row_count(2)
        .assert_table_row("backend", &[("STATE", "Created"), ("PID", "-")])
        .assert_table_row("frontend", &[("STATE", "Created"), ("PID", "-")]);

    env.cli(&["start", "backend"]).assert_success();
    env.daemon().wait_for_log_default("[backend] spawned");
    env.cli(&["start", "frontend"]).assert_success();
    env.daemon().wait_for_log_default("[frontend] spawned");

    let backend_pid = env.cli(&["list"]).pid_from_table_row("backend");
    let frontend_pid = env.cli(&["list"]).pid_from_table_row("frontend");
    assert!(
        pid_is_alive(backend_pid),
        "backend PID {backend_pid} should be alive"
    );
    assert!(
        pid_is_alive(frontend_pid),
        "frontend PID {frontend_pid} should be alive"
    );

    let out = env.cli(&["describe", "--json", "frontend"]);
    out.assert_success();
    let json = out.stdout_json();
    let after = json["after"].as_array().expect("after should be an array");
    assert!(
        after.iter().any(|v| v.as_str() == Some("backend")),
        "frontend should depend on backend: {json}"
    );
}

#[test]
fn test_cli_create_nonexistent_dependency_ignored() {
    let env = TestEnv::new().start();

    let out = env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--after",
        "does-not-exist",
    ]);
    out.assert_success();
    out.assert_stderr_contains("not found, ignoring");

    env.daemon().wait_for_log_default("[svc] spawned");

    let pid = env.cli(&["list"]).pid_from_table_row("svc");
    assert!(
        pid_is_alive(pid),
        "PID {pid} should be alive despite missing dep"
    );

    let out = env.cli(&["describe", "--json", "svc"]);
    out.assert_success();
    let json = out.stdout_json();
    let after = json["after"].as_array().expect("after should be an array");
    assert!(
        after.iter().any(|v| v.as_str() == Some("does-not-exist")),
        "after should still list the nonexistent dep: {json}"
    );
}

#[test]
fn test_cli_create_nonexistent_dependency_json_warnings() {
    let env = TestEnv::new().start();

    let out = env.cli(&[
        "create",
        "--name",
        "svc",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--after",
        "ghost",
        "--json",
    ]);
    out.assert_success();
    let json = out.stdout_json();
    let warnings = json["warnings"]
        .as_array()
        .expect("warnings should be an array");
    assert!(
        warnings.iter().any(|w| {
            let s = w.as_str().unwrap_or("");
            s.contains("ghost") && s.contains("not found")
        }),
        "JSON warnings should mention missing dep: {json}"
    );
}

#[test]
fn test_cli_all_commands_json_parseable() {
    let env = TestEnv::new()
        .with_config("svc", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[svc] spawned");

    let commands: Vec<&[&str]> = vec![
        &["status", "--json"],
        &["list", "--json"],
        &["describe", "--json", "svc"],
        &["config", "--json"],
    ];
    for args in &commands {
        let out = env.cli(args);
        out.assert_success();
        out.stdout_json();
    }

    env.cli(&["stop", "--json", "svc"])
        .assert_success()
        .stdout_json();

    env.cli(&["start", "--json", "svc"])
        .assert_success()
        .stdout_json();

    env.cli(&[
        "create",
        "--json",
        "--name",
        "dyn",
        "--command",
        "/bin/sleep",
        "--args",
        "300",
        "--no-auto-start",
    ])
    .assert_success()
    .stdout_json();

    write_config(
        env.config_dir(),
        "extra",
        "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
    );
    env.cli(&["reload", "--json"])
        .assert_success()
        .stdout_json();
}

#[test]
fn test_cli_errors_on_stderr() {
    let env = TestEnv::new().start();

    let cases: Vec<(&[&str], &str)> = vec![
        (&["describe", "nonexistent"], "not found"),
        (&["start", "nonexistent"], "not found"),
        (&["stop", "nonexistent"], "not found"),
    ];
    for (args, pattern) in &cases {
        let out = env.cli(args);
        out.assert_failure();
        out.assert_stderr_contains(pattern);
        assert!(
            out.stdout.trim().is_empty(),
            "stdout should be empty on error for {:?}, got: {}",
            args,
            out.stdout
        );
    }
}

#[test]
fn test_cli_exit_codes() {
    let env = TestEnv::new()
        .with_config("svc", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[svc] spawned");

    let success_cmds: Vec<&[&str]> =
        vec![&["status"], &["list"], &["describe", "svc"], &["config"]];
    for args in &success_cmds {
        let out = env.cli(args);
        assert_eq!(out.status.code(), Some(0), "expected exit 0 for {:?}", args);
    }

    let failure_cmds: Vec<&[&str]> = vec![
        &["describe", "nonexistent"],
        &["start", "nonexistent"],
        &["stop", "nonexistent"],
    ];
    for args in &failure_cmds {
        let out = env.cli(args);
        assert_eq!(out.status.code(), Some(1), "expected exit 1 for {:?}", args);
    }
}

#[test]
fn test_cli_restart_on_failure_ignores_success_exit() {
    let env = TestEnv::new()
        .with_config("ok", "command: /usr/bin/true\nrestart: on-failure\n")
        .start();

    env.daemon().wait_for_log_default("[ok] exited with");

    let out = env.cli(&["list", "--json"]);
    out.assert_success();
    let json = out.stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["restart_count"], 0);
}

#[test]
fn test_cli_daemon_nonexistent_config_dir() {
    let env = TestEnv::new();
    let config_dir = env.config_dir().display().to_string();
    std::fs::remove_dir(env.config_dir()).expect("failed to remove config dir");
    let env = env.start();

    env.daemon().wait_for_log_default(&format!(
        "config directory {config_dir} does not exist, no processes to manage"
    ));

    env.cli(&["status"])
        .assert_success()
        .assert_field("Ready", "true")
        .assert_field("Total Processes", "0");

    env.cli(&["list"])
        .assert_success()
        .assert_stdout_contains("No processes");
}

#[test]
fn test_cli_daemon_shutdown_stops_children() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli(&["list"]).pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "child PID {pid} should be alive");

    drop(env);

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "child PID {pid} should be gone after daemon shutdown"
    );
}

#[test]
fn test_cli_daemon_shutdown_via_sigint() {
    let env = TestEnv::new()
        .with_config("sleeper", "command: /bin/sleep\nargs:\n  - '300'\n")
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let child_pid = env.cli(&["list"]).pid_from_table_row("sleeper");
    let daemon_pid = env.daemon_pid();

    assert!(pid_is_alive(child_pid), "child should be alive");
    assert!(pid_is_alive(daemon_pid), "daemon should be alive");

    env.daemon().send_signal(nix::sys::signal::Signal::SIGINT);
    drop(env);

    assert!(
        !pid_is_alive(daemon_pid),
        "daemon PID {daemon_pid} should be gone after SIGINT"
    );
    assert!(
        !pid_is_alive(child_pid),
        "child PID {child_pid} should be gone after SIGINT shutdown"
    );
}

#[test]
fn test_cli_restart_on_success_restarts_on_exit_zero() {
    let env = TestEnv::new()
        .with_config("ok-loop", "command: /usr/bin/true\nrestart: on-success\n")
        .start();

    assert!(
        env.daemon()
            .wait_for_log_count("[ok-loop] spawned", 3, Duration::from_secs(10)),
        "on-success should restart on exit 0"
    );

    let json = env.cli(&["list", "--json"]).stdout_json();
    let count = json[0]["restart_count"].as_u64().unwrap();
    assert!(count >= 2, "expected restart_count >= 2, got {count}");
}

#[test]
fn test_cli_restart_on_failure_restarts_on_exit_nonzero() {
    let env = TestEnv::new()
        .with_config("crasher", "command: /usr/bin/false\nrestart: on-failure\n")
        .start();

    assert!(
        env.daemon()
            .wait_for_log_count("[crasher] spawned", 3, Duration::from_secs(10)),
        "on-failure should restart on exit 1"
    );

    let json = env.cli(&["list", "--json"]).stdout_json();
    let count = json[0]["restart_count"].as_u64().unwrap();
    assert!(count >= 2, "expected restart_count >= 2, got {count}");
}

#[test]
fn test_cli_restart_on_success_ignores_failure_exit() {
    let env = TestEnv::new()
        .with_config(
            "fail-once",
            "command: /usr/bin/false\nrestart: on-success\n",
        )
        .start();

    env.daemon().wait_for_log_default("[fail-once] exited with");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Failed");
    assert_eq!(json[0]["restart_count"], 0);
}

#[test]
fn test_cli_burst_limiting_stops_restarts() {
    let env = TestEnv::new()
        .with_config(
            "burst",
            concat!(
                "command: /usr/bin/false\n",
                "restart: always\n",
                "restart_sec: 2\n",
                "restart_max_delay_sec: 2\n",
                "start_limit_burst: 4\n",
                "start_limit_interval_sec: 60\n",
            ),
        )
        .start();

    env.daemon()
        .wait_for_log("[burst] start limit reached", Duration::from_secs(30));

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Failed");
    assert_eq!(json[0]["restart_count"], 4);
}

#[test]
fn test_cli_burst_interval_allows_spaced_restarts() {
    let env = TestEnv::new()
        .with_config(
            "spaced",
            concat!(
                "command: /usr/bin/false\n",
                "restart: always\n",
                "restart_sec: 0.6\n",
                "restart_max_delay_sec: 0.6\n",
                "start_limit_burst: 2\n",
                "start_limit_interval_sec: 1\n",
            ),
        )
        .start();

    assert!(
        env.daemon()
            .wait_for_log_count("[spaced] spawned", 5, Duration::from_secs(10)),
        "interval window should allow restarts to continue past burst limit"
    );

    assert_eq!(
        env.daemon().count_log_matches("start limit reached"),
        0,
        "burst limiting should never trigger when restarts are spaced"
    );

    let json = env.cli(&["list", "--json"]).stdout_json();
    let count = json[0]["restart_count"].as_u64().unwrap();
    assert!(count >= 4, "expected restart_count >= 4, got {count}");
}

#[test]
fn test_cli_condition_path_exists_not_met() {
    let env = TestEnv::new()
        .with_config(
            "missing-bin",
            "command: /nonexistent/binary\ncondition_path_exists: /nonexistent/binary\n",
        )
        .start();

    env.daemon()
        .wait_for_log_default("[missing-bin] condition_path_exists not met");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["name"], "missing-bin");
    assert_eq!(json[0]["state"], "Created");
    assert_eq!(json[0]["pid"], 0);
}

#[test]
fn test_cli_environment_file_loading() {
    let env = TestEnv::new();
    let env_file = env.config_dir().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    let env = env
        .with_config(
            "env-test",
            &format!(
                concat!(
                    "command: /bin/sh\n",
                    "args:\n",
                    "  - '-c'\n",
                    "  - 'exit $(test \"$MY_VAR\" = \"from_file\" && echo 0 || echo 1)'\n",
                    "environment_file: {}\n",
                    "restart: never\n",
                ),
                env_file.display()
            ),
        )
        .start();

    env.daemon().wait_for_log_default("[env-test] exited with");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[test]
fn test_cli_env_overrides_environment_file() {
    let env = TestEnv::new();
    let env_file = env.config_dir().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    let env = env
        .with_config(
            "override-test",
            &format!(
                concat!(
                    "command: /bin/sh\n",
                    "args:\n",
                    "  - '-c'\n",
                    "  - 'exit $(test \"$MY_VAR\" = \"overridden\" && echo 0 || echo 1)'\n",
                    "environment_file: {}\n",
                    "env:\n",
                    "  MY_VAR: overridden\n",
                    "restart: never\n",
                ),
                env_file.display()
            ),
        )
        .start();

    env.daemon()
        .wait_for_log_default("[override-test] exited with");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[test]
fn test_cli_child_does_not_inherit_parent_env() {
    let env = TestEnv::new()
        .with_config(
            "clean-env",
            concat!(
                "command: /bin/sh\n",
                "args:\n",
                "  - '-c'\n",
                "  - 'test -z \"$HOME\" && exit 0 || exit 1'\n",
                "restart: never\n",
            ),
        )
        .start();

    env.daemon().wait_for_log_default("[clean-env] exited with");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[test]
fn test_cli_optional_environment_file_skipped_when_missing() {
    let env = TestEnv::new()
        .with_config(
            "opt-env",
            concat!(
                "command: /bin/sh\n",
                "args:\n",
                "  - '-c'\n",
                "  - 'exit 0'\n",
                "environment_file: -/nonexistent/env\n",
                "restart: never\n",
            ),
        )
        .start();

    env.daemon()
        .wait_for_log_default("optional environment file not found, skipping");
    env.daemon().wait_for_log_default("[opt-env] exited with");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[test]
fn test_cli_invalid_yaml_skipped() {
    let env = TestEnv::new().with_config("good", "command: /bin/sleep\nargs:\n  - '300'\n");

    std::fs::write(env.config_dir().join("bad.yaml"), "not: valid: yaml: [").unwrap();

    let env = env.start();

    env.daemon().wait_for_log_default("[good] spawned");

    let json = env.cli(&["list", "--json"]).stdout_json();
    assert_eq!(json.as_array().unwrap().len(), 1);
    assert_eq!(json[0]["name"], "good");
}

fn render_ddot_template(
    install_dir: &str,
    etc_dir: &str,
    pid_dir: &str,
    fleet_dir: &str,
) -> Option<String> {
    let tmpl_path = std::path::PathBuf::from(option_env!("DDOT_TEMPLATE_PATH")?);
    let tmpl = std::fs::read_to_string(&tmpl_path)
        .unwrap_or_else(|e| panic!("failed to read {}: {e}", tmpl_path.display()));
    let rendered = tmpl
        .replace("{{.InstallDir}}", install_dir)
        .replace("{{.EtcDir}}", etc_dir)
        .replace("{{.PIDDir}}", pid_dir)
        .replace("{{.FleetPoliciesDir}}", fleet_dir);
    Some(
        rendered
            .lines()
            .filter(|line| !line.contains("{{"))
            .collect::<Vec<_>>()
            .join("\n"),
    )
}

#[test]
fn test_ddot_template_starts_with_env_and_optional_envfile() {
    let dir = tempfile::tempdir().unwrap();

    let install_dir = dir.path().join("install");
    let bin_dir = install_dir.join("ext/ddot/embedded/bin");
    std::fs::create_dir_all(&bin_dir).unwrap();

    let script = bin_dir.join("otel-agent");
    std::fs::write(
        &script,
        concat!(
            "#!/bin/sh\n",
            "if [ \"$DD_FLEET_POLICIES_DIR\" = \"/etc/dd/policies\" ]; then\n",
            "  exit 0\n",
            "else\n",
            "  echo \"DD_FLEET_POLICIES_DIR=$DD_FLEET_POLICIES_DIR\" >&2\n",
            "  exit 1\n",
            "fi\n",
        ),
    )
    .unwrap();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
    }

    let etc_dir = dir.path().join("etc");
    std::fs::create_dir_all(&etc_dir).unwrap();
    let pid_dir = dir.path().join("pid");
    std::fs::create_dir_all(pid_dir.join("run")).unwrap();

    let config_dir = dir.path().join("processes.d");
    std::fs::create_dir_all(&config_dir).unwrap();

    let Some(yaml) = render_ddot_template(
        install_dir.to_str().unwrap(),
        etc_dir.to_str().unwrap(),
        pid_dir.to_str().unwrap(),
        "/etc/dd/policies",
    ) else {
        eprintln!("DDOT_TEMPLATE_PATH not set at compile time, skipping");
        return;
    };
    std::fs::write(config_dir.join("datadog-agent-ddot.yaml"), &yaml).unwrap();

    let sock = dir.path().join("daemon.sock");
    let mut daemon = helpers::DaemonHandle::start(&config_dir, &sock);
    assert!(
        daemon.wait_for_log_default(
            "[datadog-agent-ddot] optional environment file not found, skipping"
        ),
        "daemon should skip the missing optional env file for ddot"
    );
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] spawned (pid="),
        "ddot process should be spawned with the otel-agent binary"
    );
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] exited with exit status: 0"),
        "ddot process should exit 0 (DD_FLEET_POLICIES_DIR env var was set correctly)"
    );
    assert!(
        daemon.wait_for_log_default(
            "[datadog-agent-ddot] exit does not match restart policy, not restarting"
        ),
        "on-failure restart should not trigger on exit 0"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_ddot_template_skipped_when_binary_missing() {
    let dir = tempfile::tempdir().unwrap();
    let config_dir = dir.path().join("processes.d");
    std::fs::create_dir_all(&config_dir).unwrap();

    let Some(yaml) = render_ddot_template(
        "/nonexistent/install",
        "/nonexistent/etc",
        "/nonexistent/pid",
        "/nonexistent/policies",
    ) else {
        eprintln!("DDOT_TEMPLATE_PATH not set at compile time, skipping");
        return;
    };
    std::fs::write(config_dir.join("datadog-agent-ddot.yaml"), &yaml).unwrap();

    let sock = dir.path().join("daemon.sock");
    let mut daemon = helpers::DaemonHandle::start(&config_dir, &sock);
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] condition_path_exists not met"),
        "daemon should skip ddot when otel-agent binary is missing"
    );
    assert_eq!(
        daemon.count_log_matches("[datadog-agent-ddot] spawned"),
        0,
        "ddot should NOT be spawned when condition_path_exists is not met"
    );

    let status = daemon.stop();
    assert!(status.success());
}
