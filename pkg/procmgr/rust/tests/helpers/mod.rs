// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(dead_code)]

use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(10);

// ---------------------------------------------------------------------------
// DaemonHandle
// ---------------------------------------------------------------------------

/// Handle to a running dd-procmgrd daemon process.
pub struct DaemonHandle {
    child: Child,
    log_lines: Arc<Mutex<Vec<String>>>,
    _reader_thread: std::thread::JoinHandle<()>,
    _stderr_thread: std::thread::JoinHandle<()>,
}

impl DaemonHandle {
    /// Start the daemon with the given config directory and socket path.
    pub fn start(config_dir: &Path, socket_path: &Path) -> Self {
        let bin = env!("CARGO_BIN_EXE_dd-procmgrd");
        let mut child = Command::new(bin)
            .env("DD_PM_CONFIG_DIR", config_dir)
            .env("DD_PM_SOCKET_PATH", socket_path)
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

    pub fn pid(&self) -> u32 {
        self.child.id()
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
    pub fn stop(&mut self) -> ExitStatus {
        self.send_signal(Signal::SIGTERM);
        self.wait_with_timeout(DEFAULT_TIMEOUT)
    }

    /// Wait for the daemon to exit within the given timeout.
    pub fn wait_with_timeout(&mut self, timeout: Duration) -> ExitStatus {
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

// ---------------------------------------------------------------------------
// CliOutput
// ---------------------------------------------------------------------------

/// Captured output from a dd-procmgr CLI invocation.
pub struct CliOutput {
    pub status: ExitStatus,
    pub stdout: String,
    pub stderr: String,
}

impl CliOutput {
    pub fn assert_success(&self) -> &Self {
        assert!(
            self.status.success(),
            "expected exit 0, got {:?}\nstdout: {}\nstderr: {}",
            self.status.code(),
            self.stdout,
            self.stderr,
        );
        self
    }

    pub fn assert_failure(&self) -> &Self {
        assert!(
            !self.status.success(),
            "expected non-zero exit, got 0\nstdout: {}\nstderr: {}",
            self.stdout,
            self.stderr,
        );
        self
    }

    pub fn assert_stdout_contains(&self, pattern: &str) -> &Self {
        assert!(
            self.stdout.contains(pattern),
            "stdout does not contain '{pattern}'\nstdout: {}",
            self.stdout,
        );
        self
    }

    pub fn assert_stderr_contains(&self, pattern: &str) -> &Self {
        assert!(
            self.stderr.contains(pattern),
            "stderr does not contain '{pattern}'\nstderr: {}",
            self.stderr,
        );
        self
    }

    /// Parse the table header to find column positions, locate the row where
    /// the NAME column equals `row_name`, and assert each (column, expected)
    /// pair matches.
    pub fn assert_table_row(&self, row_name: &str, expected: &[(&str, &str)]) -> &Self {
        let lines: Vec<&str> = self.stdout.lines().collect();
        assert!(
            !lines.is_empty(),
            "table output is empty\nstdout: {}",
            self.stdout
        );

        let header = lines[0];
        let col_starts = parse_table_columns(header);

        let row = lines[1..]
            .iter()
            .find(|line| {
                let val = extract_column(line, &col_starts, "NAME");
                val == Some(row_name)
            })
            .unwrap_or_else(|| {
                panic!(
                    "no table row with NAME='{row_name}'\nstdout: {}",
                    self.stdout
                )
            });

        for &(col, exp) in expected {
            let actual = extract_column(row, &col_starts, col)
                .unwrap_or_else(|| panic!("column '{col}' not found in header: {header}"));
            assert_eq!(
                actual, exp,
                "row '{row_name}' column '{col}': expected '{exp}', got '{actual}'\nfull row: {row}",
            );
        }
        self
    }

    /// Assert the table has exactly `n` data rows (excluding the header).
    pub fn assert_table_row_count(&self, n: usize) -> &Self {
        let data_rows = self.stdout.lines().count().saturating_sub(1);
        assert_eq!(
            data_rows, n,
            "expected {n} table rows, got {data_rows}\nstdout: {}",
            self.stdout,
        );
        self
    }

    /// Parse "Label:  value" lines and assert the field matches.
    pub fn assert_field(&self, label: &str, expected: &str) -> &Self {
        let needle = format!("{label}:");
        let value = self
            .stdout
            .lines()
            .find_map(|line| {
                let trimmed = line.trim();
                if trimmed.starts_with(&needle) {
                    Some(trimmed[needle.len()..].trim().to_string())
                } else {
                    None
                }
            })
            .unwrap_or_else(|| {
                panic!(
                    "field '{label}' not found in output\nstdout: {}",
                    self.stdout
                )
            });
        assert_eq!(
            value, expected,
            "field '{label}': expected '{expected}', got '{value}'",
        );
        self
    }

    /// Assert that a field label exists regardless of value.
    pub fn assert_has_field(&self, label: &str) -> &Self {
        let needle = format!("{label}:");
        assert!(
            self.stdout
                .lines()
                .any(|line| line.trim().starts_with(&needle)),
            "field '{label}' not found in output\nstdout: {}",
            self.stdout,
        );
        self
    }

    /// Parse stdout as JSON.
    pub fn stdout_json(&self) -> serde_json::Value {
        serde_json::from_str(&self.stdout).unwrap_or_else(|e| {
            panic!(
                "failed to parse stdout as JSON: {e}\nstdout: {}",
                self.stdout
            )
        })
    }

    /// Extract PID from a key-value field as u32.
    pub fn pid_from_field(&self, label: &str) -> u32 {
        let needle = format!("{label}:");
        let value = self
            .stdout
            .lines()
            .find_map(|line| {
                let trimmed = line.trim();
                if trimmed.starts_with(&needle) {
                    Some(trimmed[needle.len()..].trim().to_string())
                } else {
                    None
                }
            })
            .unwrap_or_else(|| panic!("field '{label}' not found\nstdout: {}", self.stdout));
        value
            .parse::<u32>()
            .unwrap_or_else(|e| panic!("field '{label}' is not a valid PID: '{value}': {e}"))
    }

    /// Extract PID for a row in table output as u32.
    pub fn pid_from_table_row(&self, row_name: &str) -> u32 {
        let lines: Vec<&str> = self.stdout.lines().collect();
        assert!(!lines.is_empty(), "table output is empty");

        let col_starts = parse_table_columns(lines[0]);
        let row = lines[1..]
            .iter()
            .find(|line| extract_column(line, &col_starts, "NAME") == Some(row_name))
            .unwrap_or_else(|| {
                panic!(
                    "no table row with NAME='{row_name}'\nstdout: {}",
                    self.stdout
                )
            });

        let pid_str = extract_column(row, &col_starts, "PID")
            .unwrap_or_else(|| panic!("PID column not found"));
        pid_str
            .parse::<u32>()
            .unwrap_or_else(|e| panic!("PID is not a valid number: '{pid_str}': {e}"))
    }
}

/// Parse table header to find (column_name, start_index) pairs.
///
/// Columns are separated by 2+ spaces. A single space between uppercase words
/// is treated as a multi-word header (e.g. "LAST EXIT"). This mirrors the
/// padding logic in dd-procmgr's table formatter — if the output format
/// changes, this parser must be updated to match.
fn parse_table_columns(header: &str) -> Vec<(String, usize)> {
    let mut cols = Vec::new();
    let mut i = 0;
    let bytes = header.as_bytes();
    while i < bytes.len() {
        if bytes[i] == b' ' {
            i += 1;
            continue;
        }
        let start = i;
        while i < bytes.len() && bytes[i] != b' ' {
            i += 1;
        }
        // Peek ahead: if the next non-space char is uppercase and follows
        // exactly 2 spaces, this might be a multi-word header like "LAST EXIT".
        let saved_i = i;
        let mut spaces = 0;
        let mut peek = i;
        while peek < bytes.len() && bytes[peek] == b' ' {
            spaces += 1;
            peek += 1;
        }
        if spaces == 1 && peek < bytes.len() && bytes[peek].is_ascii_uppercase() {
            // Multi-word column header (e.g. "LAST EXIT")
            i = peek;
            while i < bytes.len() && bytes[i] != b' ' {
                i += 1;
            }
            cols.push((header[start..i].to_string(), start));
        } else {
            i = saved_i;
            cols.push((header[start..i].to_string(), start));
        }
    }
    cols
}

/// Extract a column value from a table row using column positions.
fn extract_column<'a>(row: &'a str, cols: &[(String, usize)], name: &str) -> Option<&'a str> {
    let idx = cols.iter().position(|(n, _)| n == name)?;
    let start = cols[idx].1;
    let end = if idx + 1 < cols.len() {
        cols[idx + 1].1
    } else {
        row.len()
    };
    if start >= row.len() {
        return Some("");
    }
    let end = end.min(row.len());
    Some(row[start..end].trim())
}

// ---------------------------------------------------------------------------
// CliRunner
// ---------------------------------------------------------------------------

/// Runs dd-procmgr CLI commands against a daemon socket.
pub struct CliRunner {
    socket_path: PathBuf,
}

impl CliRunner {
    pub fn new(socket_path: &Path) -> Self {
        Self {
            socket_path: socket_path.to_path_buf(),
        }
    }

    /// Run a dd-procmgr command and capture output.
    pub fn run(&self, args: &[&str]) -> CliOutput {
        let bin = env!("CARGO_BIN_EXE_dd-procmgr");
        let output = Command::new(bin)
            .arg("--socket")
            .arg(&self.socket_path)
            .args(args)
            .output()
            .expect("failed to run dd-procmgr");

        CliOutput {
            status: output.status,
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        }
    }

    /// Run a dd-procmgr command with --json prepended.
    pub fn run_json(&self, args: &[&str]) -> CliOutput {
        let bin = env!("CARGO_BIN_EXE_dd-procmgr");
        let output = Command::new(bin)
            .arg("--socket")
            .arg(&self.socket_path)
            .arg("--json")
            .args(args)
            .output()
            .expect("failed to run dd-procmgr");

        CliOutput {
            status: output.status,
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        }
    }
}

// ---------------------------------------------------------------------------
// TestEnv
// ---------------------------------------------------------------------------

/// Self-contained test environment: temp dir, daemon, and CLI runner.
/// Drop stops the daemon and cleans up the temp dir.
pub struct TestEnv {
    _dir: tempfile::TempDir,
    config_dir: PathBuf,
    socket_path: PathBuf,
    daemon: Option<DaemonHandle>,
}

impl TestEnv {
    pub fn new() -> Self {
        let dir = tempfile::tempdir().expect("failed to create temp dir");
        let config_dir = dir.path().join("processes.d");
        std::fs::create_dir_all(&config_dir).expect("failed to create config dir");
        let socket_path = dir.path().join("daemon.sock");
        Self {
            _dir: dir,
            config_dir,
            socket_path,
            daemon: None,
        }
    }

    /// Write a process YAML config into the config dir.
    pub fn with_config(self, name: &str, yaml: &str) -> Self {
        write_config(&self.config_dir, name, yaml);
        self
    }

    /// Start the daemon and wait until gRPC is ready.
    pub fn start(mut self) -> Self {
        let daemon = DaemonHandle::start(&self.config_dir, &self.socket_path);
        assert!(
            daemon.wait_for_log_default("gRPC server listening on"),
            "daemon gRPC server should be ready"
        );
        self.daemon = Some(daemon);
        self
    }

    /// Run a CLI command against this environment's daemon.
    pub fn cli(&self, args: &[&str]) -> CliOutput {
        let runner = CliRunner::new(&self.socket_path);
        runner.run(args)
    }

    /// Run a CLI command with --json against this environment's daemon.
    pub fn cli_json(&self, args: &[&str]) -> CliOutput {
        let runner = CliRunner::new(&self.socket_path);
        runner.run_json(args)
    }

    /// Access the daemon handle for log inspection, PID checks, etc.
    pub fn daemon(&self) -> &DaemonHandle {
        self.daemon.as_ref().expect("daemon not started")
    }

    /// Get the daemon's own PID.
    pub fn daemon_pid(&self) -> u32 {
        self.daemon().pid()
    }

    /// Write a new config file (for reload tests).
    pub fn add_config(&self, name: &str, yaml: &str) {
        write_config(&self.config_dir, name, yaml);
    }

    /// Remove a config file (for reload tests).
    pub fn remove_config(&self, name: &str) {
        let path = self.config_dir.join(format!("{name}.yaml"));
        std::fs::remove_file(&path)
            .unwrap_or_else(|e| panic!("failed to remove {}: {e}", path.display()));
    }
}

impl Drop for TestEnv {
    fn drop(&mut self) {
        if let Some(ref mut daemon) = self.daemon {
            let _ = daemon.stop();
        }
    }
}

// ---------------------------------------------------------------------------
// Free functions
// ---------------------------------------------------------------------------

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
