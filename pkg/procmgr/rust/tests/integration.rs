// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod helpers;

use helpers::{DaemonHandle, pid_is_alive, wait_for_pid_gone, write_config};
use std::time::Duration;

// ===========================================================================
// Group 1: Basic Lifecycle
// ===========================================================================

#[test]
fn test_daemon_starts_and_spawns_process() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "sleeper",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("spawned"),
        "daemon should log spawned"
    );

    let pids = daemon.spawned_pids();
    assert_eq!(pids.len(), 1, "expected 1 spawned process");
    assert!(pid_is_alive(pids[0]), "managed process should be alive");

    let status = daemon.stop();
    assert!(status.success(), "daemon should exit cleanly");
    assert!(
        wait_for_pid_gone(pids[0], Duration::from_secs(5)),
        "managed process should be gone after shutdown"
    );
}

#[test]
fn test_daemon_no_config_dir() {
    let dir = tempfile::tempdir().unwrap();
    let nonexistent = dir.path().join("nonexistent");

    let mut daemon = DaemonHandle::start(&nonexistent);
    assert!(
        daemon.wait_for_log_default("does not exist"),
        "daemon should log missing config dir"
    );

    let status = daemon.stop();
    assert!(status.success(), "daemon should exit cleanly");
}

#[test]
fn test_daemon_empty_config_dir() {
    let dir = tempfile::tempdir().unwrap();

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("loaded 0 process config(s)"),
        "daemon should log zero configs"
    );

    let status = daemon.stop();
    assert!(status.success(), "daemon should exit cleanly");
}

// ===========================================================================
// Group 2: Graceful Shutdown
// ===========================================================================

#[test]
fn test_shutdown_stops_managed_processes() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "sleep1",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );
    write_config(
        dir.path(),
        "sleep2",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log("loaded 2 process config(s)", Duration::from_secs(5)),
        "daemon should load 2 configs"
    );
    assert!(
        daemon.wait_for_log_count("spawned", 2, Duration::from_secs(5)),
        "daemon should spawn 2 processes"
    );

    let pids = daemon.spawned_pids();
    assert_eq!(pids.len(), 2);
    for &pid in &pids {
        assert!(pid_is_alive(pid), "pid {pid} should be alive before shutdown");
    }

    let status = daemon.stop();
    assert!(status.success(), "daemon should exit cleanly");

    for &pid in &pids {
        assert!(
            wait_for_pid_gone(pid, Duration::from_secs(5)),
            "pid {pid} should be gone after shutdown"
        );
    }
}

#[test]
fn test_shutdown_sends_sigterm_to_children() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "sleeper",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("spawned"),
        "daemon should spawn the process"
    );

    let pids = daemon.spawned_pids();
    assert_eq!(pids.len(), 1);

    let status = daemon.stop();
    assert!(
        daemon.wait_for_log("sending SIGTERM", Duration::from_secs(0)),
        "daemon should log sending SIGTERM during shutdown"
    );
    assert!(status.success(), "daemon should exit cleanly");
}

// ===========================================================================
// Group 3: Restart Policies
// ===========================================================================

#[test]
fn test_restart_always_on_failure() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "crasher",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 1'\nrestart: always\nrestart_sec: 0.1\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_count("spawned", 3, Duration::from_secs(10)),
        "process should be restarted at least 3 times, got {}",
        daemon.count_log_matches("spawned")
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_restart_never() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "once",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 0'\nrestart: never\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with"),
        "process should exit"
    );
    // Wait a bit to make sure no restart happens.
    std::thread::sleep(Duration::from_secs(1));
    assert_eq!(
        daemon.count_log_matches("spawned"),
        1,
        "process should NOT be restarted"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_restart_on_failure_with_success_exit() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "success",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 0'\nrestart: on-failure\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with"),
        "process should exit"
    );
    std::thread::sleep(Duration::from_secs(1));
    assert_eq!(
        daemon.count_log_matches("spawned"),
        1,
        "on-failure should NOT restart on success exit"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_restart_on_success() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "ok-loop",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 0'\nrestart: on-success\nrestart_sec: 0.1\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_count("spawned", 3, Duration::from_secs(10)),
        "on-success should restart on exit 0, got {} spawns",
        daemon.count_log_matches("spawned")
    );

    let status = daemon.stop();
    assert!(status.success());
}

// ===========================================================================
// Group 4: Backoff and Burst Limiting
// ===========================================================================

#[test]
fn test_burst_limiting_stops_restarts() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "burst",
        concat!(
            "command: /bin/sh\n",
            "args:\n  - '-c'\n  - 'exit 1'\n",
            "restart: always\n",
            "restart_sec: 0.05\n",
            "start_limit_burst: 3\n",
            "start_limit_interval_sec: 60\n",
        ),
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log("start limit reached", Duration::from_secs(10)),
        "daemon should log start limit reached"
    );

    // After start limit, no more restarts. Initial spawn + 3 restarts = 4 total max.
    std::thread::sleep(Duration::from_secs(1));
    let total_spawns = daemon.count_log_matches("spawned");
    assert!(
        total_spawns <= 4,
        "expected at most 4 spawns (1 initial + 3 burst), got {total_spawns}"
    );

    let status = daemon.stop();
    assert!(status.success());
}

// ===========================================================================
// Group 5: Condition and Config
// ===========================================================================

#[test]
fn test_condition_path_exists_not_met() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "missing-bin",
        "command: /nonexistent/binary\ncondition_path_exists: /nonexistent/binary\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("condition_path_exists not met"),
        "daemon should log condition not met"
    );

    assert_eq!(
        daemon.count_log_matches("spawned"),
        0,
        "process should NOT be spawned"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_auto_start_false() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "disabled",
        "command: /bin/sleep\nargs:\n  - '300'\nauto_start: false\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("auto_start=false, skipping"),
        "daemon should log auto_start skip"
    );

    assert_eq!(
        daemon.count_log_matches("spawned"),
        0,
        "process should NOT be spawned"
    );

    let status = daemon.stop();
    assert!(status.success());
}

// ===========================================================================
// Group 6: Additional restart policy combinations
// ===========================================================================

#[test]
fn test_restart_on_failure_with_failure_exit() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "crasher",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 1'\nrestart: on-failure\nrestart_sec: 0.1\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_count("spawned", 3, Duration::from_secs(10)),
        "on-failure should restart on failure exit, got {} spawns",
        daemon.count_log_matches("spawned")
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_restart_on_success_with_failure_exit() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "fail-once",
        "command: /bin/sh\nargs:\n  - '-c'\n  - 'exit 1'\nrestart: on-success\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with"),
        "process should exit"
    );
    std::thread::sleep(Duration::from_secs(1));
    assert_eq!(
        daemon.count_log_matches("spawned"),
        1,
        "on-success should NOT restart on failure exit"
    );

    let status = daemon.stop();
    assert!(status.success());
}

// ===========================================================================
// Group 7: SIGINT shutdown
// ===========================================================================

#[test]
fn test_shutdown_via_sigint() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "sleeper",
        "command: /bin/sleep\nargs:\n  - '300'\n",
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("spawned"),
        "daemon should spawn the process"
    );

    let pids = daemon.spawned_pids();
    assert_eq!(pids.len(), 1);

    daemon.send_signal(nix::sys::signal::Signal::SIGINT);
    let status = daemon.wait_with_timeout(Duration::from_secs(10));

    assert!(
        daemon.wait_for_log("received SIGINT", Duration::from_secs(0)),
        "daemon should log received SIGINT"
    );
    assert!(status.success(), "daemon should exit cleanly on SIGINT");
}

// ===========================================================================
// Group 8: Environment handling
// ===========================================================================

#[test]
fn test_environment_file_loading() {
    let dir = tempfile::tempdir().unwrap();
    let env_file = dir.path().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    write_config(
        dir.path(),
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
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with exit status: 0"),
        "child should see env var from environment_file"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_env_overrides_environment_file() {
    let dir = tempfile::tempdir().unwrap();
    let env_file = dir.path().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    write_config(
        dir.path(),
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
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with exit status: 0"),
        "per-process env should override environment_file"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[test]
fn test_child_does_not_inherit_parent_env() {
    let dir = tempfile::tempdir().unwrap();
    write_config(
        dir.path(),
        "clean-env",
        concat!(
            "command: /bin/sh\n",
            "args:\n",
            "  - '-c'\n",
            "  - 'test -z \"$HOME\" && exit 0 || exit 1'\n",
            "restart: never\n",
        ),
    );

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("exited with exit status: 0"),
        "child should NOT inherit parent env (HOME should be unset)"
    );

    let status = daemon.stop();
    assert!(status.success());
}

// ===========================================================================
// Group 9: Error handling
// ===========================================================================

#[test]
fn test_spawn_failure_logged_and_skipped() {
    let dir = tempfile::tempdir().unwrap();
    write_config(dir.path(), "bad-bin", "command: /nonexistent/binary\n");

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("failed to spawn"),
        "daemon should log spawn failure"
    );
    assert_eq!(
        daemon.count_log_matches("spawned (pid="),
        0,
        "no process should be spawned"
    );

    let status = daemon.stop();
    assert!(
        status.success(),
        "daemon should keep running despite spawn failure"
    );
}

#[test]
fn test_invalid_yaml_skipped() {
    let dir = tempfile::tempdir().unwrap();
    std::fs::write(
        dir.path().join("good.yaml"),
        "command: /bin/sleep\nargs:\n  - '300'\n",
    )
    .unwrap();
    std::fs::write(dir.path().join("bad.yaml"), "not: valid: yaml: [").unwrap();

    let mut daemon = DaemonHandle::start(dir.path());
    assert!(
        daemon.wait_for_log_default("loaded 1 process config(s)"),
        "daemon should load only the valid config"
    );
    assert!(
        daemon.wait_for_log_default("spawned"),
        "valid process should be spawned"
    );

    let status = daemon.stop();
    assert!(status.success());
}
