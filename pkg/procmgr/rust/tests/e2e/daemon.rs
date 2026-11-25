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

/// Creates a named pipe at `path`, reporting the OS error on failure.
#[cfg(unix)]
fn make_fifo(path: &std::path::Path) {
    use std::os::unix::ffi::OsStrExt;
    let c = std::ffi::CString::new(path.as_os_str().as_bytes()).expect("path has no NUL");
    // SAFETY: `c` is a NUL-terminated path that outlives the call, and mkfifo does not keep the
    // pointer.
    let rc = unsafe { libc::mkfifo(c.as_ptr(), 0o600) };
    assert_eq!(
        rc,
        0,
        "mkfifo {}: {}",
        path.display(),
        std::io::Error::last_os_error()
    );
}

/// Owns the thread that opens a FIFO for writing. Dropping the guard releases the writer and
/// waits for it, so a panic in the test does not leave the thread blocked in open(2).
#[cfg(unix)]
struct FifoWriter {
    path: std::path::PathBuf,
    release: Option<std::sync::mpsc::Sender<()>>,
    handle: Option<std::thread::JoinHandle<()>>,
}

#[cfg(unix)]
impl FifoWriter {
    /// Spawns a thread that opens `path` for writing, reports on `opened`, waits for `finish`,
    /// then runs `body` with the open file.
    fn spawn(
        path: std::path::PathBuf,
        opened: std::sync::mpsc::Sender<()>,
        body: impl FnOnce(std::fs::File) + Send + 'static,
    ) -> Self {
        let (release, release_rx) = std::sync::mpsc::channel::<()>();
        let open_path = path.clone();
        let handle = std::thread::spawn(move || {
            let file = std::fs::OpenOptions::new()
                .write(true)
                .open(&open_path)
                .expect("open FIFO for writing");
            let _ = opened.send(());
            // A dropped sender makes recv fail, so the writer returns without writing.
            if release_rx.recv().is_ok() {
                body(file);
            }
        });
        Self {
            path,
            release: Some(release),
            handle: Some(handle),
        }
    }

    /// Releases the writer and waits for it. Use on the normal path.
    fn finish(&mut self) {
        if let Some(release) = self.release.take() {
            let _ = release.send(());
        }
        if let Some(handle) = self.handle.take() {
            handle.join().expect("writer thread panicked");
        }
    }
}

#[cfg(unix)]
impl Drop for FifoWriter {
    fn drop(&mut self) {
        use std::os::unix::fs::OpenOptionsExt;
        // Dropping the sender releases the writer without writing.
        drop(self.release.take());
        // After finish() there is nothing left to unblock or join.
        let Some(handle) = self.handle.take() else {
            return;
        };
        // Keep the read end open until the writer is joined, so a writer still in open(2)
        // completes.
        let _reader = std::fs::OpenOptions::new()
            .read(true)
            .custom_flags(libc::O_NONBLOCK)
            .open(&self.path);
        let _ = handle.join();
    }
}

/// Kills a child's process group when dropped, unless disarmed. The child leads its own group.
#[cfg(unix)]
struct KillGroupOnDrop(Option<u32>);

#[cfg(unix)]
impl KillGroupOnDrop {
    /// Stops the guard from killing the group. Call it once the child has exited, since the PID
    /// may be reused after that.
    fn disarm(&mut self) {
        self.0 = None;
    }
}

#[cfg(unix)]
impl Drop for KillGroupOnDrop {
    fn drop(&mut self) {
        if let Some(pid) = self.0 {
            // A pid of 0 or 1 would name this test's own group or every process we can signal.
            if pid > 1 {
                // SAFETY: a negative PID targets only the process group led by the child.
                unsafe { libc::kill(-(pid as libc::pid_t), libc::SIGKILL) };
            }
        }
    }
}

/// PID of a spawned process, read from its `[name] spawned (pid=N, ...)` log line.
#[cfg(unix)]
fn spawned_pid(daemon: &DaemonHandle, name: &str) -> u32 {
    let prefix = format!("[{name}] spawned (pid=");
    let line = daemon
        .captured_logs()
        .into_iter()
        .find(|l| l.contains(&prefix))
        .unwrap_or_else(|| panic!("no spawn line for {name}"));
    let rest = &line[line.find(&prefix).expect("prefix") + prefix.len()..];
    rest.split(|c: char| !c.is_ascii_digit())
        .next()
        .expect("pid digits")
        .parse()
        .expect("pid is a number")
}

/// A SIGTERM during startup must end the daemon cleanly and start nothing after the gate. The FIFO
/// gate holds startup open until the test releases it after the signal. A process started before
/// the gate must be stopped during teardown. This covers the signal-during-startup path only. The
/// window before the runtime exists (the early handler) needs a PID 1 or exec-based test.
#[cfg(unix)]
#[test]
fn test_cli_daemon_shutdown_during_startup_exits_cleanly() {
    use nix::sys::signal::Signal;
    use std::os::unix::process::ExitStatusExt;

    let env = TestEnv::new();
    let gate = env.env_root().join("gate.fifo");
    make_fifo(&gate);
    let env = env
        .with_config("earlier", &format!("{}auto_start: true\n", test_helpers::sleep_config_yaml()))
        .with_config(
            "gated",
            &format!(
                "command: {}\nauto_start: true\nafter:\n  - earlier\ncondition_config_any:\n  - path: {}\n    keys:\n      - apm_config.enabled\n",
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
    let (opened_tx, opened_rx) = std::sync::mpsc::channel::<()>();
    let mut gate_writer = FifoWriter::spawn(gate.clone(), opened_tx, |mut writer| {
        // Open the gate, so "gated" would start if the daemon did not check for the stop. The
        // daemon may have exited already, so a closed reader is not an error here.
        let _ = std::io::Write::write_all(&mut writer, b"apm_config:\n  enabled: true\n");
    });
    // Declared before the daemon, so on a panic the daemon is stopped before the child.
    let mut kill_earlier = KillGroupOnDrop(None);
    let socket = env.env_root().join("daemon.sock");
    let mut daemon = DaemonHandle::start(env.config_dir(), &socket);
    // The open returns once the daemon reads the gate, which stays blocked while the writer
    // is open.
    opened_rx
        .recv_timeout(Duration::from_secs(10))
        .unwrap_or_else(|e| {
            panic!(
                "daemon never opened the gate fifo ({e})\n{}",
                daemon.captured_logs().join("\n")
            )
        });
    // "earlier" is ordered before the gate, so it has spawned. Its log line reaches the capture
    // on a reader thread, so wait for it before reading the PID.
    assert!(
        daemon.wait_for_log("[earlier] spawned", Duration::from_secs(10)),
        "earlier never spawned\n{}",
        daemon.captured_logs().join("\n")
    );
    let earlier_pid = spawned_pid(&daemon, "earlier");
    // The daemon handle only kills the daemon, so kill the child's process group if a later
    // assertion panics. The guard is disarmed once the child is confirmed gone.
    kill_earlier.0 = Some(earlier_pid);
    assert!(
        pid_is_alive(earlier_pid),
        "earlier PID {earlier_pid} should be alive"
    );

    daemon.send_signal(Signal::SIGTERM);
    // Wait until the watcher logs the signal, then let the gate read finish with the gate open.
    // The start pass must still see the stop before it spawns anything.
    assert!(
        daemon.wait_for_log("received SIGTERM", Duration::from_secs(10)),
        "daemon did not record the stop request\n{}",
        daemon.captured_logs().join("\n")
    );
    gate_writer.finish();
    let status = daemon
        .wait_for_exit(Duration::from_secs(10))
        .expect("daemon did not exit after SIGTERM during the start pass");
    // Disarm the group guard only once the child is confirmed gone, so a failed assertion below
    // cannot signal a group whose PID may have been reused.
    let earlier_gone = wait_for_pid_gone(earlier_pid, Duration::from_secs(5));
    if earlier_gone {
        kill_earlier.disarm();
    }
    assert_eq!(
        status.code(),
        Some(0),
        "daemon should exit 0, got {status:?}"
    );
    assert_eq!(status.signal(), None, "daemon was killed by a signal");
    assert!(
        daemon.wait_for_log("dd-procmgrd stopped (exit code 0)", Duration::from_secs(10)),
        "daemon did not log its final stop line\n{}",
        daemon.captured_logs().join("\n")
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
    assert!(
        earlier_gone,
        "earlier PID {earlier_pid} should be stopped by teardown after a stop during startup"
    );
}

/// A critical process that fails to spawn before a startup stop keeps its failure exit code. The
/// gated process after it holds the start pass open until the test sends SIGTERM.
#[cfg(unix)]
#[test]
fn test_cli_daemon_startup_stop_keeps_critical_spawn_failure_exit_code() {
    use nix::sys::signal::Signal;
    use std::os::unix::process::ExitStatusExt;

    let env = TestEnv::new();
    let gate = env.env_root().join("gate.fifo");
    make_fifo(&gate);
    let env = env
        .with_config(
            "critical",
            "command: /nonexistent/dd-procmgr-missing-binary\nauto_start: true\nexit_with_supervisor: true\n",
        )
        .with_config(
            "gated",
            &format!(
                "command: {}\nauto_start: true\nafter:\n  - critical\ncondition_config_any:\n  - path: {}\n    keys:\n      - apm_config.enabled\n",
                test_helpers::true_cmd().0,
                gate.display()
            ),
        );
    let (opened_tx, opened_rx) = std::sync::mpsc::channel::<()>();
    let mut gate_writer = FifoWriter::spawn(gate.clone(), opened_tx, |mut writer| {
        let _ = std::io::Write::write_all(&mut writer, b"apm_config:\n  enabled: true\n");
    });
    let socket = env.env_root().join("daemon.sock");
    let mut daemon = DaemonHandle::start(env.config_dir(), &socket);
    opened_rx
        .recv_timeout(Duration::from_secs(10))
        .unwrap_or_else(|e| {
            panic!(
                "daemon never opened the gate fifo ({e})\n{}",
                daemon.captured_logs().join("\n")
            )
        });
    assert!(
        daemon.wait_for_log("critical process failed to start", Duration::from_secs(10)),
        "critical spawn failure was not logged\n{}",
        daemon.captured_logs().join("\n")
    );

    daemon.send_signal(Signal::SIGTERM);
    assert!(
        daemon.wait_for_log("received SIGTERM", Duration::from_secs(10)),
        "daemon did not record the stop request\n{}",
        daemon.captured_logs().join("\n")
    );
    gate_writer.finish();
    let status = daemon
        .wait_for_exit(Duration::from_secs(10))
        .expect("daemon did not exit after SIGTERM");
    // 1 is the critical spawn failure exit code. A stop must not replace it with 0.
    assert_eq!(
        status.code(),
        Some(1),
        "daemon should keep the critical spawn failure exit code, got {status:?}"
    );
    assert_eq!(status.signal(), None, "daemon was killed by a signal");
    // The reader threads may still hold the last lines after the exit, so wait for the final one.
    assert!(
        daemon.wait_for_log("dd-procmgrd stopped (exit code 1)", Duration::from_secs(10)),
        "daemon did not log its final stop line\n{}",
        daemon.captured_logs().join("\n")
    );
    assert_eq!(
        daemon.count_log_matches("[gated] spawned"),
        0,
        "gated process started after a stop during the start pass"
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
    make_fifo(&blocked);
    // The writer opens the FIFO, which returns once the loader opens it for reading. It then
    // waits for the release, so the daemon is still inside ProcessManager::new when SIGTERM lands.
    let (opened_tx, opened_rx) = std::sync::mpsc::channel::<()>();
    let mut catalog_writer = FifoWriter::spawn(blocked.clone(), opened_tx, |mut writer| {
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
        .unwrap_or_else(|e| {
            panic!(
                "daemon never opened the catalog fifo ({e})\n{}",
                daemon.captured_logs().join("\n")
            )
        });

    daemon.send_signal(Signal::SIGTERM);
    // Wait until the watcher logs the signal, then let the catalog read finish. The loaded
    // process must still not start.
    assert!(
        daemon.wait_for_log("received SIGTERM", Duration::from_secs(10)),
        "daemon did not record the stop request\n{}",
        daemon.captured_logs().join("\n")
    );
    catalog_writer.finish();
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
        "daemon did not log its final stop line\n{}",
        daemon.captured_logs().join("\n")
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
