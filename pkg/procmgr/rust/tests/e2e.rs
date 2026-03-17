// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod helpers;

use helpers::{CliRunner, TestEnv, pid_is_alive};
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
    env.daemon().wait_for_log_default("[svc-a] stopped");

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
