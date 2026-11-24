// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(unix)]
use crate::helpers::DaemonHandle;
use crate::helpers::{StatusProcessesCount, TestEnv, pid_is_alive, wait_for_pid_gone};
use dd_procmgrd::test_helpers;
use std::time::Duration;

#[test]
fn daemon_starts_ready() {
    let env = TestEnv::new();
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_version_not_empty();
    status.assert_processes_count(StatusProcessesCount::zeros());
}

#[test]
fn status_fails_without_daemon() {
    let env = TestEnv::new();
    env.status()
        .expect_err("expected status to fail without daemon");
}

#[test]
fn status_ready_without_config_dir() {
    let env = TestEnv::with_missing_config_dir();
    let procmgr = env.start();
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount::zeros());
}

#[test]
fn test_cli_daemon_nonexistent_config_dir() {
    let env = TestEnv::with_missing_config_dir();
    let config_dir = env.config_dir().display().to_string();
    let env = env.start();

    env.daemon().wait_for_log_default(&format!(
        "config directory {config_dir} does not exist, no processes to manage"
    ));

    env.cli_status()
        .assert_success()
        .assert_field("Ready", "true")
        .assert_field("Total Processes", "0");

    env.cli_list()
        .assert_success()
        .assert_stdout_contains("No processes");
}

#[test]
fn test_cli_daemon_shutdown_stops_children() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let pid = env.cli_list().pid_from_table_row("sleeper");
    assert!(pid_is_alive(pid), "child PID {pid} should be alive");

    drop(env);

    assert!(
        wait_for_pid_gone(pid, Duration::from_secs(5)),
        "child PID {pid} should be gone after daemon shutdown"
    );
}

/// A SIGTERM during startup must end the daemon cleanly and start nothing after the gate. The FIFO
/// gate holds startup open until the test releases it after the signal.
#[cfg(unix)]
#[test]
fn test_cli_daemon_shutdown_during_startup_exits_cleanly() {
    use nix::sys::signal::Signal;
    use std::os::unix::process::ExitStatusExt;

    let env = TestEnv::new();
    let gate = env.env_root().join("gate.fifo");
    let gate_c = std::ffi::CString::new(gate.to_str().expect("utf-8 path")).expect("no NUL");
    assert_eq!(
        unsafe { libc::mkfifo(gate_c.as_ptr(), 0o600) },
        0,
        "mkfifo failed"
    );
    let env = env
        .with_config(
            "gated",
            &format!(
                "command: {}\nauto_start: true\ncondition_config_any:\n  - path: {}\n    keys:\n      - apm_config.enabled\n",
                test_helpers::true_cmd().0,
                gate.display()
            ),
        )
        // Ordered after the gated process, so the start pass reaches it only after the gate.
        .with_config(
            "later",
            &format!(
                "command: {}\nauto_start: true\nafter:\n  - gated\n",
                test_helpers::true_cmd().0
            ),
        );
    // A helper thread opens the FIFO for writing. The blocking open returns once the daemon
    // opens it for reading, and the daemon's gate read then waits until the thread drops the
    // writer. Dropping it early would leave a daemon that has not opened yet blocked forever.
    let (release_tx, release_rx) = std::sync::mpsc::channel::<()>();
    let (opened_tx, opened_rx) = std::sync::mpsc::channel::<()>();
    let writer_path = gate.clone();
    let writer = std::thread::spawn(move || {
        let mut writer = std::fs::OpenOptions::new()
            .write(true)
            .open(&writer_path)
            .expect("open gate fifo for writing");
        let _ = opened_tx.send(());
        let _ = release_rx.recv();
        // Open the gate, so "gated" would start if the daemon did not check for the stop. The
        // daemon may have exited already, so a closed reader is not an error here.
        let _ = std::io::Write::write_all(&mut writer, b"apm_config:\n  enabled: true\n");
        drop(writer);
    });
    let socket = env.env_root().join("daemon.sock");
    let mut daemon = DaemonHandle::start(env.config_dir(), &socket);
    // The open returns once the daemon reads the gate, which stays blocked while the writer
    // is open.
    opened_rx
        .recv_timeout(Duration::from_secs(10))
        .expect("daemon never opened the gate fifo");

    daemon.send_signal(Signal::SIGTERM);
    // Give the handler time to record the stop, then let the gate read finish with the gate
    // open. The start pass must still see the stop before it spawns anything.
    std::thread::sleep(Duration::from_millis(300));
    let _ = release_tx.send(());
    writer.join().expect("writer thread panicked");
    let status = daemon
        .wait_for_exit(Duration::from_secs(10))
        .expect("daemon did not exit after SIGTERM during the start pass");
    assert_eq!(
        status.code(),
        Some(0),
        "daemon should exit 0, got {status:?}"
    );
    assert_eq!(status.signal(), None, "daemon was killed by a signal");
    assert!(
        daemon.wait_for_log("dd-procmgrd stopped (exit code 0)", Duration::from_secs(10)),
        "daemon did not log its final stop line"
    );
    // The gate opened, so "gated" would have started without the check after the gate read.
    assert_eq!(
        daemon.count_log_matches("[gated] spawned"),
        0,
        "a gated process started after a stop during the start pass"
    );
    assert_eq!(
        daemon.count_log_matches("[later] spawned"),
        0,
        "a process after the gate started after a stop during the start pass"
    );
    assert_eq!(
        daemon.count_log_matches("stop requested during startup"),
        1,
        "the pending stop should end the start pass"
    );
}

/// A SIGTERM while the catalog loads must start nothing. A FIFO catalog entry blocks the loader
/// until the test releases it after the signal.
#[cfg(unix)]
#[test]
fn test_cli_daemon_shutdown_during_catalog_load_starts_nothing() {
    use nix::sys::signal::Signal;
    use std::os::unix::process::ExitStatusExt;

    let env = TestEnv::new();
    let blocked = env.config_dir().join("blocked.yaml");
    let blocked_c = std::ffi::CString::new(blocked.to_str().expect("utf-8 path")).expect("no NUL");
    assert_eq!(
        unsafe { libc::mkfifo(blocked_c.as_ptr(), 0o600) },
        0,
        "mkfifo failed"
    );

    // The writer opens the FIFO, which returns once the loader opens it for reading. It then
    // waits for the release, so the daemon is still inside ProcessManager::new when SIGTERM lands.
    let (opened_tx, opened_rx) = std::sync::mpsc::channel::<()>();
    let (release_tx, release_rx) = std::sync::mpsc::channel::<()>();
    let writer_path = blocked.clone();
    let writer = std::thread::spawn(move || {
        let mut writer = std::fs::OpenOptions::new()
            .write(true)
            .open(&writer_path)
            .expect("open catalog fifo for writing");
        let _ = opened_tx.send(());
        let _ = release_rx.recv();
        let config = format!(
            "command: {}\nauto_start: true\n",
            test_helpers::true_cmd().0
        );
        let _ = std::io::Write::write_all(&mut writer, config.as_bytes());
    });

    let socket = env.env_root().join("daemon.sock");
    let mut daemon = DaemonHandle::start(env.config_dir(), &socket);
    opened_rx
        .recv_timeout(Duration::from_secs(10))
        .expect("daemon never opened the catalog fifo");

    daemon.send_signal(Signal::SIGTERM);
    // Give the handler time to record the stop, then let the catalog read finish. The loaded
    // process must still not start.
    std::thread::sleep(Duration::from_millis(300));
    let _ = release_tx.send(());
    writer.join().expect("writer thread panicked");
    let status = daemon
        .wait_for_exit(Duration::from_secs(10))
        .expect("daemon did not exit after SIGTERM during catalog load");
    assert_eq!(
        status.code(),
        Some(0),
        "daemon should exit 0, got {status:?}"
    );
    assert_eq!(status.signal(), None, "daemon was killed by a signal");
    // The reader threads may still hold the last lines after the exit, so wait for the final one.
    assert!(
        daemon.wait_for_log("dd-procmgrd stopped (exit code 0)", Duration::from_secs(10)),
        "daemon did not log its final stop line"
    );
    assert_eq!(
        daemon.count_log_matches("[blocked] spawned"),
        0,
        "a process started after a stop during catalog load"
    );
    assert_eq!(
        daemon.count_log_matches("stop requested before startup"),
        1,
        "the pending stop should skip the start pass"
    );
}

#[cfg(unix)]
#[test]
fn test_cli_daemon_shutdown_via_sigint() {
    let env = TestEnv::new()
        .with_config("sleeper", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[sleeper] spawned");

    let child_pid = env.cli_list().pid_from_table_row("sleeper");
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
