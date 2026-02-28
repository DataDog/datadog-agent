// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::io::{BufRead, BufReader};
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(10);

/// Handle to a running dd-procmgrd daemon process.
pub struct DaemonHandle {
    child: Child,
    log_lines: Arc<Mutex<Vec<String>>>,
    _reader_thread: std::thread::JoinHandle<()>,
    _stderr_thread: std::thread::JoinHandle<()>,
}

impl DaemonHandle {
    /// Start the daemon with `DD_PM_CONFIG_DIR` pointing to the given directory.
    pub fn start(config_dir: &Path) -> Self {
        let bin = env!("CARGO_BIN_EXE_dd-procmgrd");
        let mut child = Command::new(bin)
            .env("DD_PM_CONFIG_DIR", config_dir)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("failed to start dd-procmgrd");

        let stdout = child.stdout.take().expect("failed to capture stdout");
        let stderr = child.stderr.take().expect("failed to capture stderr");
        let log_lines = Arc::new(Mutex::new(Vec::<String>::new()));
        let lines_clone = Arc::clone(&log_lines);
        let lines_clone2 = Arc::clone(&log_lines);

        // simple_logger writes INFO to stdout, WARN/ERROR to stderr.
        let reader_thread = std::thread::spawn(move || {
            let reader = BufReader::new(stdout);
            for line in reader.lines() {
                match line {
                    Ok(l) => {
                        eprintln!("[daemon] {l}");
                        lines_clone.lock().unwrap().push(l);
                    }
                    Err(_) => break,
                }
            }
        });

        let _stderr_thread = std::thread::spawn(move || {
            let reader = BufReader::new(stderr);
            for line in reader.lines() {
                match line {
                    Ok(l) => {
                        eprintln!("[daemon:err] {l}");
                        lines_clone2.lock().unwrap().push(l);
                    }
                    Err(_) => break,
                }
            }
        });

        Self {
            child,
            log_lines,
            _reader_thread: reader_thread,
            _stderr_thread,
        }
    }

    /// Wait until a log line containing `pattern` appears, or timeout.
    pub fn wait_for_log(&self, pattern: &str, timeout: Duration) -> bool {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let lines = self.log_lines.lock().unwrap();
                if lines.iter().any(|l| l.contains(pattern)) {
                    return true;
                }
            }
            if Instant::now() >= deadline {
                return false;
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    /// Wait until a log line containing `pattern` appears using the default timeout.
    pub fn wait_for_log_default(&self, pattern: &str) -> bool {
        self.wait_for_log(pattern, DEFAULT_TIMEOUT)
    }

    /// Count how many log lines contain `pattern`.
    pub fn count_log_matches(&self, pattern: &str) -> usize {
        let lines = self.log_lines.lock().unwrap();
        lines.iter().filter(|l| l.contains(pattern)).count()
    }

    /// Wait until the count of log lines matching `pattern` reaches at least `n`.
    pub fn wait_for_log_count(&self, pattern: &str, n: usize, timeout: Duration) -> bool {
        let deadline = Instant::now() + timeout;
        loop {
            if self.count_log_matches(pattern) >= n {
                return true;
            }
            if Instant::now() >= deadline {
                return false;
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    /// Send a signal to the daemon process.
    pub fn send_signal(&self, sig: Signal) {
        let pid = self.child.id() as i32;
        signal::kill(Pid::from_raw(pid), sig).expect("failed to send signal to daemon");
    }

    /// Send SIGTERM and wait for the daemon to exit. Returns the exit status.
    pub fn stop(&mut self) -> std::process::ExitStatus {
        self.send_signal(Signal::SIGTERM);
        self.wait_with_timeout(DEFAULT_TIMEOUT)
    }

    /// Wait for the daemon to exit within the given timeout.
    pub fn wait_with_timeout(&mut self, timeout: Duration) -> std::process::ExitStatus {
        let deadline = Instant::now() + timeout;
        loop {
            match self
                .child
                .try_wait()
                .expect("failed to check daemon status")
            {
                Some(status) => return status,
                None => {
                    if Instant::now() >= deadline {
                        self.child.kill().ok();
                        return self.child.wait().expect("failed to wait on killed daemon");
                    }
                    std::thread::sleep(Duration::from_millis(50));
                }
            }
        }
    }

    /// Extract PIDs from "spawned (pid=NNN" log lines.
    pub fn spawned_pids(&self) -> Vec<u32> {
        let lines = self.log_lines.lock().unwrap();
        lines
            .iter()
            .filter_map(|l| {
                let marker = "spawned (pid=";
                let start = l.find(marker)? + marker.len();
                let end = l[start..].find(|c: char| !c.is_ascii_digit())? + start;
                l[start..end].parse().ok()
            })
            .collect()
    }
}

impl Drop for DaemonHandle {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

/// Write a YAML config file into `dir` with the given process `name`.
pub fn write_config(dir: &Path, name: &str, yaml: &str) {
    let path = dir.join(format!("{name}.yaml"));
    std::fs::write(&path, yaml)
        .unwrap_or_else(|e| panic!("failed to write {}: {e}", path.display()));
}

/// Check if a PID is still alive.
pub fn pid_is_alive(pid: u32) -> bool {
    signal::kill(Pid::from_raw(pid as i32), None).is_ok()
}

/// Wait until a PID is no longer alive, or timeout.
pub fn wait_for_pid_gone(pid: u32, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    loop {
        if !pid_is_alive(pid) {
            return true;
        }
        if Instant::now() >= deadline {
            return false;
        }
        std::thread::sleep(Duration::from_millis(50));
    }
}
