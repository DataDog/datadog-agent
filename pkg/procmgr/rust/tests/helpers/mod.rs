// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(unix)]
use nix::sys::signal::{self, Signal};
#[cfg(unix)]
use nix::unistd::Pid;
use serde::Deserialize;
use std::collections::BTreeMap;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(10);
const DEFAULT_START_TIMEOUT: Duration = Duration::from_secs(30);
const DEFAULT_PROCESS_WAIT_TIMEOUT: Duration = Duration::from_secs(30);
const DEFAULT_STABLE_RUNNING: Duration = Duration::from_secs(5);
const START_TIMEOUT_ENV: &str = "PROCMGR_TEST_START_TIMEOUT_SECS";
const PROCESS_WAIT_TIMEOUT_ENV: &str = "PROCMGR_TEST_PROCESS_WAIT_TIMEOUT_SECS";
const STABLE_RUNNING_ENV: &str = "PROCMGR_TEST_STABLE_RUNNING_SECS";

#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct DaemonStatus {
    pub ready: bool,
    pub version: String,
    pub uptime_seconds: u64,
    pub total_processes: u32,
    pub running_processes: u32,
    pub created_processes: u32,
    pub stopped_processes: u32,
    pub failed_processes: u32,
    pub exited_processes: u32,
    pub starting_processes: u32,
    pub stopping_processes: u32,
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct ProcessSnapshot {
    pub uuid: String,
    pub name: String,
    pub state: String,
    pub pid: u64,
    #[serde(default)]
    pub profile: String,
    #[serde(default)]
    pub user: String,
    pub command: String,
    pub args: Vec<String>,
    pub restart_count: u64,
    pub last_exit_code: Option<i32>,
    pub last_signal: Option<i32>,
}

/// Parsed output from `list --json`.
#[derive(Debug, Clone, PartialEq)]
pub struct ProcessList(pub Vec<ProcessSnapshot>);

#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
struct ReloadSnapshot {
    added: Vec<String>,
    removed: Vec<String>,
    modified: Vec<String>,
    unchanged: Vec<String>,
}

/// Unset list fields are not checked. An empty vec requires the list to be empty.
/// A non-empty vec requires every name to appear in the list.
#[derive(Debug, Clone, Default)]
pub struct ReloadExpect {
    pub added: Option<Vec<String>>,
    pub removed: Option<Vec<String>>,
    pub modified: Option<Vec<String>>,
    pub unchanged: Option<Vec<String>>,
    /// Running processes whose PID must not change across reload.
    pub preserve_running_pids: Vec<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ReloadList {
    Added,
    Removed,
    Modified,
    Unchanged,
}

impl ReloadList {
    fn label(self) -> &'static str {
        match self {
            Self::Added => "added",
            Self::Removed => "removed",
            Self::Modified => "modified",
            Self::Unchanged => "unchanged",
        }
    }

    fn names(self, snapshot: &ReloadSnapshot) -> &[String] {
        match self {
            Self::Added => &snapshot.added,
            Self::Removed => &snapshot.removed,
            Self::Modified => &snapshot.modified,
            Self::Unchanged => &snapshot.unchanged,
        }
    }
}

impl ReloadSnapshot {
    fn assert_matches(&self, expected: &ReloadExpect) {
        Self::assert_list_expect(self, ReloadList::Added, &expected.added);
        Self::assert_list_expect(self, ReloadList::Removed, &expected.removed);
        Self::assert_list_expect(self, ReloadList::Modified, &expected.modified);
        Self::assert_list_expect(self, ReloadList::Unchanged, &expected.unchanged);
    }

    fn assert_list_expect(
        snapshot: &ReloadSnapshot,
        list: ReloadList,
        expected: &Option<Vec<String>>,
    ) {
        let Some(expected) = expected else {
            return;
        };
        let actual = list.names(snapshot);
        if expected.is_empty() {
            assert!(
                actual.is_empty(),
                "expected reload {} to be empty, got {snapshot:?}",
                list.label()
            );
            return;
        }
        for name in expected {
            assert!(
                actual.iter().any(|n| n == name),
                "expected '{name}' in reload {}, got {snapshot:?}",
                list.label()
            );
        }
    }
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
struct DescribeSnapshot {
    pub uuid: String,
    pub name: String,
    #[serde(default)]
    pub description: String,
    pub state: String,
    pub pid: u64,
    #[serde(default)]
    pub profile: String,
    #[serde(default)]
    pub user: String,
    pub command: String,
    pub args: Vec<String>,
    #[serde(default)]
    pub working_dir: String,
    #[serde(default)]
    pub env: BTreeMap<String, String>,
    #[serde(default)]
    pub restart_policy: String,
    pub restart_count: u64,
    pub last_exit_code: Option<i32>,
    pub last_signal: Option<i32>,
    pub auto_start: bool,
    #[serde(default)]
    pub stdout: String,
    #[serde(default)]
    pub stderr: String,
    #[serde(default)]
    pub condition_path_exists: String,
    #[serde(default)]
    pub after: Vec<String>,
    #[serde(default)]
    pub before: Vec<String>,
    #[serde(default)]
    pub runtime_user: String,
}

/// Unset fields are not checked (same pattern as ReloadExpect / StatusProcessesCount).
#[derive(Debug, Clone, Default)]
pub struct DescribeExpect {
    pub name: Option<String>,
    pub state: Option<String>,
    pub uuid: Option<String>,
    pub pid: Option<u64>,
    pub command: Option<String>,
    pub args: Option<Vec<String>>,
    pub description: Option<String>,
    pub working_dir: Option<String>,
    pub restart_policy: Option<String>,
    pub auto_start: Option<bool>,
    pub restart_count: Option<u64>,
    pub restart_count_at_least: Option<u64>,
    pub last_exit_code: Option<Option<i32>>,
    pub env_contains: Option<BTreeMap<String, String>>,
    pub after: Option<Vec<String>>,
    pub has_uuid: Option<bool>,
    pub has_stdout_path: Option<bool>,
    pub has_stderr_path: Option<bool>,
    pub pid_alive: Option<bool>,
}

impl DescribeSnapshot {
    fn assert_matches(&self, expected: &DescribeExpect) {
        assert_describe_field("name", &self.name, &expected.name, self);
        assert_describe_field("state", &self.state, &expected.state, self);
        assert_describe_field("uuid", &self.uuid, &expected.uuid, self);
        assert_describe_field("pid", &self.pid, &expected.pid, self);
        assert_describe_field("command", &self.command, &expected.command, self);
        assert_describe_field("args", &self.args, &expected.args, self);
        assert_describe_field(
            "description",
            &self.description,
            &expected.description,
            self,
        );
        assert_describe_field(
            "working_dir",
            &self.working_dir,
            &expected.working_dir,
            self,
        );
        assert_describe_field(
            "restart_policy",
            &self.restart_policy,
            &expected.restart_policy,
            self,
        );
        assert_describe_field("auto_start", &self.auto_start, &expected.auto_start, self);
        assert_describe_field(
            "restart_count",
            &self.restart_count,
            &expected.restart_count,
            self,
        );
        if let Some(min) = expected.restart_count_at_least {
            assert!(
                self.restart_count >= min,
                "describe restart_count: expected >={min}, got {}\nfull: {self:?}",
                self.restart_count
            );
        }
        assert_describe_field(
            "last_exit_code",
            &self.last_exit_code,
            &expected.last_exit_code,
            self,
        );
        if let Some(expected_env) = &expected.env_contains {
            for (key, value) in expected_env {
                assert_eq!(
                    self.env.get(key),
                    Some(value),
                    "describe env[{key}]: expected {value:?}, got {:?}\nfull: {self:?}",
                    self.env.get(key)
                );
            }
        }
        if let Some(expected_after) = &expected.after {
            if expected_after.is_empty() {
                assert!(
                    self.after.is_empty(),
                    "expected describe after to be empty, got {self:?}"
                );
            } else {
                for name in expected_after {
                    assert!(
                        self.after.iter().any(|n| n == name),
                        "expected '{name}' in describe after, got {self:?}"
                    );
                }
            }
        }
        assert_describe_present("uuid", &self.uuid, expected.has_uuid, self);
        assert_describe_present("stdout", &self.stdout, expected.has_stdout_path, self);
        assert_describe_present("stderr", &self.stderr, expected.has_stderr_path, self);
        if let Some(expected_alive) = expected.pid_alive {
            let alive = self.pid > 0 && pid_is_alive(self.pid as u32);
            assert_eq!(
                alive,
                expected_alive,
                "describe pid_alive: expected {expected_alive}, pid {} alive={}\nfull: {self:?}",
                self.pid,
                pid_is_alive(self.pid as u32)
            );
        }
    }
}

fn assert_describe_field<T: PartialEq + std::fmt::Debug>(
    field: &str,
    actual: &T,
    expected: &Option<T>,
    snapshot: &DescribeSnapshot,
) {
    if let Some(expected) = expected {
        assert_eq!(
            actual, expected,
            "describe {field}: expected {expected:?}, got {actual:?}\nfull: {snapshot:?}"
        );
    }
}

fn assert_describe_present(
    field: &str,
    actual: &str,
    expected: Option<bool>,
    snapshot: &DescribeSnapshot,
) {
    match expected {
        Some(true) => assert!(
            !actual.is_empty(),
            "describe {field} should be present, got {snapshot:?}"
        ),
        Some(false) => assert!(
            actual.is_empty(),
            "describe {field} should be empty, got {snapshot:?}"
        ),
        None => {}
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessExpect {
    Created,
    Running,
    Stopped,
    Failed,
    Exited,
}

impl ProcessExpect {
    fn as_str(self) -> &'static str {
        match self {
            Self::Created => "Created",
            Self::Running => "Running",
            Self::Stopped => "Stopped",
            Self::Failed => "Failed",
            Self::Exited => "Exited",
        }
    }
}

impl ProcessList {
    pub fn require_process(&self, name: &str) -> &ProcessSnapshot {
        self.0
            .iter()
            .find(|p| p.name == name)
            .unwrap_or_else(|| panic!("process '{name}' not found in catalog: {:?}", self.0))
    }

    pub fn assert_empty(&self) {
        assert!(
            self.0.is_empty(),
            "expected empty catalog, got {:?}",
            self.0
        );
    }

    pub fn assert_len(&self, expected: usize) {
        assert_eq!(
            self.0.len(),
            expected,
            "expected {expected} process(es) in catalog, got {:?}",
            self.0
        );
    }

    pub fn assert_absent(&self, name: &str) {
        assert!(
            !self.0.iter().any(|p| p.name == name),
            "process '{name}' should not be in catalog, got {:?}",
            self.0
        );
    }

    pub fn assert_process_state(&self, name: &str, expected: ProcessExpect) {
        let process = self.require_process(name);
        let expected_state = expected.as_str();
        assert_eq!(
            process.state, expected_state,
            "process '{name}': expected state {expected_state}, got {process:?}"
        );
        assert!(
            process_matches_expect(process, expected),
            "process '{name}' PID expectation not met for {expected_state}: {process:?}"
        );
    }

    pub fn assert_last_exit_code(&self, name: &str, code: i32) {
        let process = self.require_process(name);
        assert_eq!(
            process.last_exit_code,
            Some(code),
            "process '{name}' last_exit_code: expected {code}, got {process:?}"
        );
    }

    pub fn len(&self) -> usize {
        self.0.len()
    }
}

/// Unset fields are not checked.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct StatusProcessesCount {
    pub total: Option<u32>,
    pub running: Option<u32>,
    pub created: Option<u32>,
    pub stopped: Option<u32>,
    pub failed: Option<u32>,
    pub exited: Option<u32>,
    pub starting: Option<u32>,
    pub stopping: Option<u32>,
}

impl StatusProcessesCount {
    pub fn zeros() -> Self {
        Self {
            total: Some(0),
            running: Some(0),
            created: Some(0),
            stopped: Some(0),
            failed: Some(0),
            exited: Some(0),
            starting: Some(0),
            stopping: Some(0),
        }
    }
}

impl DaemonStatus {
    pub fn assert_ready(&self) {
        assert!(self.ready, "expected daemon ready, got {self:?}");
    }

    pub fn assert_version_not_empty(&self) {
        assert!(
            !self.version.is_empty(),
            "expected non-empty status version, got {self:?}"
        );
    }

    pub fn assert_processes_count(&self, expected: StatusProcessesCount) {
        let fields = [
            ("total_processes", self.total_processes, expected.total),
            (
                "running_processes",
                self.running_processes,
                expected.running,
            ),
            (
                "created_processes",
                self.created_processes,
                expected.created,
            ),
            (
                "stopped_processes",
                self.stopped_processes,
                expected.stopped,
            ),
            ("failed_processes", self.failed_processes, expected.failed),
            ("exited_processes", self.exited_processes, expected.exited),
            (
                "starting_processes",
                self.starting_processes,
                expected.starting,
            ),
            (
                "stopping_processes",
                self.stopping_processes,
                expected.stopping,
            ),
        ];
        for (field, actual, exp) in fields {
            assert_status_field(field, actual, exp, self);
        }
    }
}

fn duration_from_env(env: &str, default: Duration) -> Duration {
    match std::env::var(env) {
        Ok(raw) => match raw.parse::<u64>() {
            Ok(secs) => Duration::from_secs(secs),
            Err(_) => {
                eprintln!("invalid {env}={raw:?}, using {default:?}");
                default
            }
        },
        Err(_) => default,
    }
}

fn default_start_timeout() -> Duration {
    duration_from_env(START_TIMEOUT_ENV, DEFAULT_START_TIMEOUT)
}

fn default_process_wait_timeout() -> Duration {
    duration_from_env(PROCESS_WAIT_TIMEOUT_ENV, DEFAULT_PROCESS_WAIT_TIMEOUT)
}

fn stable_running_duration() -> Duration {
    duration_from_env(STABLE_RUNNING_ENV, DEFAULT_STABLE_RUNNING)
}

struct StatusClient {
    runner: CliRunner,
}

impl StatusClient {
    fn new(socket_path: &Path) -> Self {
        Self {
            runner: CliRunner::new(socket_path),
        }
    }

    fn status(&self) -> Result<DaemonStatus, String> {
        let out = self.runner.run(&["status", "--json"]);
        if !out.status.success() {
            return Err(format!(
                "status --json failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        serde_json::from_str(&out.stdout)
            .map_err(|e| format!("failed to parse status JSON: {e}\nstdout: {}", out.stdout))
    }
}

struct ListClient {
    runner: CliRunner,
}

impl ListClient {
    fn new(socket_path: &Path) -> Self {
        Self {
            runner: CliRunner::new(socket_path),
        }
    }

    fn list(&self) -> Result<Vec<ProcessSnapshot>, String> {
        let out = self.runner.run(&["list", "--json"]);
        if !out.status.success() {
            return Err(format!(
                "list --json failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        serde_json::from_str(&out.stdout)
            .map_err(|e| format!("failed to parse list JSON: {e}\nstdout: {}", out.stdout))
    }
}

struct DescribeClient {
    runner: CliRunner,
}

impl DescribeClient {
    fn new(socket_path: &Path) -> Self {
        Self {
            runner: CliRunner::new(socket_path),
        }
    }

    fn describe(&self, name_or_uuid: &str) -> Result<DescribeSnapshot, String> {
        let out = self.runner.run(&["describe", "--json", name_or_uuid]);
        if !out.status.success() {
            return Err(format!(
                "describe --json {name_or_uuid} failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        serde_json::from_str(&out.stdout)
            .map_err(|e| format!("failed to parse describe JSON: {e}\nstdout: {}", out.stdout))
    }
}

// ---------------------------------------------------------------------------
// DaemonHandle
// ---------------------------------------------------------------------------

/// Handle to a running dd-procmgrd daemon.
pub struct DaemonHandle {
    child: Child,
    log_lines: Arc<Mutex<Vec<String>>>,
    _reader_thread: std::thread::JoinHandle<()>,
    _stderr_thread: std::thread::JoinHandle<()>,
}

impl DaemonHandle {
    /// Start the daemon with the given config directory and socket path.
    /// Sets `DD_PM_CONFIG_DIR` and `DD_PM_SOCKET_PATH` environment variables.
    pub fn start(config_dir: &Path, socket_path: &Path) -> Self {
        Self::start_with_env(config_dir, socket_path, &[])
    }

    /// Like [`start`](Self::start), but also sets the given extra environment variables on the
    /// daemon process.
    pub fn start_with_env(
        config_dir: &Path,
        socket_path: &Path,
        extra_env: &[(&str, &str)],
    ) -> Self {
        let bin = env!("CARGO_BIN_EXE_dd-procmgrd");

        let mut cmd = Command::new(bin);
        cmd.env("DD_PM_CONFIG_DIR", config_dir)
            .env("DD_PM_SOCKET_PATH", socket_path)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        for (k, v) in extra_env {
            cmd.env(k, v);
        }
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt as _;
            use windows_sys::Win32::System::Threading::CREATE_NEW_PROCESS_GROUP;
            cmd.creation_flags(CREATE_NEW_PROCESS_GROUP);
        }
        let mut child = cmd.spawn().expect("failed to start daemon");

        let stdout = child.stdout.take().expect("failed to capture stdout");
        let stderr = child.stderr.take().expect("failed to capture stderr");
        let log_lines = Arc::new(Mutex::new(Vec::<String>::new()));

        let _reader_thread = spawn_log_reader(stdout, "daemon", Arc::clone(&log_lines));
        let _stderr_thread = spawn_log_reader(stderr, "daemon:err", Arc::clone(&log_lines));

        Self {
            child,
            log_lines,
            _reader_thread,
            _stderr_thread,
        }
    }

    pub fn pid(&self) -> u32 {
        self.child.id()
    }

    /// Wait until a captured log line contains all `patterns`, or timeout.
    pub fn wait_for_log_line_contains(&self, patterns: &[&str], timeout: Duration) -> bool {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let lines = self.log_lines.lock().unwrap();
                if lines
                    .iter()
                    .any(|line| patterns.iter().all(|pattern| line.contains(pattern)))
                {
                    return true;
                }
            }
            if Instant::now() >= deadline {
                return false;
            }
            std::thread::sleep(Duration::from_millis(50));
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

    /// Snapshot of captured stdout/stderr lines (timeout diagnostics).
    pub fn captured_logs(&self) -> Vec<String> {
        self.log_lines.lock().unwrap().clone()
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

    /// Send a Unix signal to the daemon process.
    #[cfg(unix)]
    pub fn send_signal(&self, sig: Signal) {
        let pid = self.child.id() as i32;
        signal::kill(Pid::from_raw(pid), sig).expect("failed to send signal to daemon");
    }

    /// Gracefully stop the daemon and wait for exit.
    pub fn stop(&mut self) -> ExitStatus {
        #[cfg(unix)]
        self.send_signal(Signal::SIGTERM);
        // TODO(S19): replace with GenerateConsoleCtrlEvent for graceful shutdown;
        // child.kill() is a force-kill placeholder until the Windows platform
        // module implements proper signal delivery.
        #[cfg(windows)]
        {
            use windows_sys::Win32::System::Console::{CTRL_BREAK_EVENT, GenerateConsoleCtrlEvent};
            let pid = self.child.id();
            let ok = unsafe { GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid) };
            if ok == 0 {
                eprintln!(
                    "GenerateConsoleCtrlEvent(CTRL_BREAK, {pid}) failed: {}, falling back to kill",
                    std::io::Error::last_os_error()
                );
                let _ = self.child.kill();
            }
        }
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

    pub fn assert_stderr_contains(&self, pattern: &str) -> &Self {
        assert!(
            self.stderr.contains(pattern),
            "stderr does not contain '{pattern}'\nstderr: {}",
            self.stderr,
        );
        self
    }

    /// Parse "Label:  value" lines and assert the field matches.
    pub fn assert_field(&self, label: &str, expected: &str) -> &Self {
        let value = self.field_value(label);
        assert_eq!(
            value, expected,
            "field '{label}': expected '{expected}', got '{value}'",
        );
        self
    }

    /// Parse a numeric "Label: value" line and assert it is at least `min`.
    pub fn assert_field_at_least(&self, label: &str, min: u64) -> &Self {
        let raw = self.field_value(label);
        let value: u64 = raw.parse().unwrap_or_else(|_| {
            panic!(
                "field '{label}': expected a number, got '{raw}'\nstdout: {}",
                self.stdout
            )
        });
        assert!(
            value >= min,
            "field '{label}': expected >={min}, got {value}\nstdout: {}",
            self.stdout
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

    pub fn field_value(&self, label: &str) -> String {
        let needle = format!("{label}:");
        self.stdout
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
            })
    }

    pub fn pid_from_field(&self, label: &str) -> u32 {
        let val = self.field_value(label);
        val.parse::<u32>()
            .unwrap_or_else(|_| panic!("PID field '{label}' value '{val}' is not a u32"))
    }

    pub fn stdout_json(&self) -> serde_json::Value {
        serde_json::from_str(&self.stdout).unwrap_or_else(|e| {
            panic!(
                "failed to parse stdout as JSON: {e}\nstdout: {}",
                self.stdout
            )
        })
    }

    pub fn assert_stdout_contains(&self, pattern: &str) -> &Self {
        assert!(
            self.stdout.contains(pattern),
            "stdout does not contain '{pattern}'\nstdout: {}",
            self.stdout,
        );
        self
    }

    /// Find a table row by NAME and assert that each (column, expected) pair matches.
    /// The header is the first line of stdout; columns are identified by their
    /// header positions (supports multi-word headers like "LAST EXIT").
    pub fn assert_table_row(&self, row_name: &str, expected: &[(&str, &str)]) -> &Self {
        let (columns, rows) = self.parse_table();
        let row = self.find_table_row(row_name, &columns, &rows);
        for &(col_name, expected_val) in expected {
            let col_idx = columns
                .iter()
                .position(|&(name, _)| name == col_name)
                .unwrap_or_else(|| panic!("column '{col_name}' not in header"));
            let actual = extract_column(row, col_idx, &columns);
            assert_eq!(
                actual, expected_val,
                "row '{row_name}', column '{col_name}': expected '{expected_val}', got '{actual}'",
            );
        }
        self
    }

    pub fn assert_table_row_count(&self, n: usize) -> &Self {
        let (_, rows) = self.parse_table();
        assert_eq!(
            rows.len(),
            n,
            "expected {n} table rows, got {}\nstdout: {}",
            rows.len(),
            self.stdout,
        );
        self
    }

    pub fn pid_from_table_row(&self, row_name: &str) -> u32 {
        let (columns, rows) = self.parse_table();
        let row = self.find_table_row(row_name, &columns, &rows);
        let pid_idx = columns
            .iter()
            .position(|&(name, _)| name == "PID")
            .expect("no PID column");
        let val = extract_column(row, pid_idx, &columns);
        val.parse::<u32>()
            .unwrap_or_else(|_| panic!("PID '{val}' is not a u32 for row '{row_name}'"))
    }

    fn find_table_row<'a>(
        &self,
        row_name: &str,
        columns: &[(&str, usize)],
        rows: &[&'a str],
    ) -> &'a str {
        rows.iter()
            .find(|r| extract_column(r, 0, columns).as_str() == row_name)
            .unwrap_or_else(|| {
                panic!(
                    "row '{row_name}' not found in table\nstdout: {}",
                    self.stdout
                )
            })
    }

    fn parse_table(&self) -> (Vec<(&str, usize)>, Vec<&str>) {
        let mut lines = self.stdout.lines();
        let header = lines
            .next()
            .unwrap_or_else(|| panic!("empty stdout, expected table header"));
        let columns = parse_table_columns(header);
        let rows: Vec<&str> = lines.filter(|l| !l.trim().is_empty()).collect();
        (columns, rows)
    }
}

/// Detect column start positions from a table header line.
/// Handles multi-word headers (e.g. "LAST EXIT") by matching known names
/// before falling back to whitespace-delimited tokens.
fn parse_table_columns(header: &str) -> Vec<(&str, usize)> {
    let known_multi_word = ["LAST EXIT"];
    let mut cols: Vec<(&str, usize)> = Vec::new();
    let mut masked = header.to_string();

    for name in &known_multi_word {
        if let Some(pos) = header.find(name) {
            cols.push((name, pos));
            masked.replace_range(pos..pos + name.len(), &" ".repeat(name.len()));
        }
    }

    let bytes = masked.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i].is_ascii_uppercase() {
            let start = i;
            while i < bytes.len() && (bytes[i].is_ascii_uppercase() || bytes[i] == b'_') {
                i += 1;
            }
            if !cols.iter().any(|&(_, p)| p == start) {
                cols.push((&header[start..i], start));
            }
        } else {
            i += 1;
        }
    }

    cols.sort_by_key(|&(_, pos)| pos);
    cols
}

fn extract_column(row: &str, col_idx: usize, columns: &[(&str, usize)]) -> String {
    let start = columns[col_idx].1;
    let end = if col_idx + 1 < columns.len() {
        columns[col_idx + 1].1
    } else {
        row.len()
    };
    row.get(start..end).unwrap_or("").trim().to_string()
}

// ---------------------------------------------------------------------------
// CliRunner
// ---------------------------------------------------------------------------

/// Runs dd-procmgr CLI commands against a daemon socket.
struct CliRunner {
    socket_path: PathBuf,
}

impl CliRunner {
    fn new(socket_path: &Path) -> Self {
        Self {
            socket_path: socket_path.to_path_buf(),
        }
    }

    /// Run a dd-procmgr command and capture output.
    fn run(&self, args: &[&str]) -> CliOutput {
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
}

// ---------------------------------------------------------------------------
// TestEnv
// ---------------------------------------------------------------------------

pub struct TestEnv {
    _dir: tempfile::TempDir,
    config_dir: PathBuf,
    socket_path: PathBuf,
    daemon: Option<DaemonHandle>,
}

impl TestEnv {
    pub fn new() -> Self {
        Self::new_inner(true)
    }

    pub fn with_missing_config_dir() -> Self {
        Self::new_inner(false)
    }

    fn new_inner(create_config_dir: bool) -> Self {
        let dir = tempfile::tempdir().expect("failed to create temp dir");
        let config_dir = dir.path().join("processes.d");
        if create_config_dir {
            std::fs::create_dir_all(&config_dir).expect("failed to create config dir");
        }
        let socket_path = dir.path().join("daemon.sock");
        Self {
            _dir: dir,
            config_dir,
            socket_path,
            daemon: None,
        }
    }

    pub fn with_config(self, name: &str, yaml: &str) -> Self {
        write_config(&self.config_dir, name, yaml);
        self
    }

    pub fn with_process(self, name: &str) -> Self {
        install_process_fixture(self.env_root(), &self.config_dir, name);
        self
    }

    pub fn install_fixture(&self, name: &str) {
        install_process_fixture(self.env_root(), &self.config_dir, name);
    }

    /// Render `fixture` into `{name}.yaml` (simulates operator replacing a process file).
    pub fn overwrite_with_fixture(&self, name: &str, fixture: &str) {
        install_process_fixture_as(self.env_root(), &self.config_dir, fixture, name);
    }

    pub fn remove_process_yaml(&self, name: &str) {
        let path = self.config_dir.join(format!("{name}.yaml"));
        std::fs::remove_file(&path)
            .unwrap_or_else(|e| panic!("failed to remove {}: {e}", path.display()));
    }

    pub fn env_root(&self) -> &Path {
        self._dir.path()
    }

    pub fn config_dir(&self) -> &Path {
        &self.config_dir
    }

    pub fn start(self) -> Self {
        self.start_with_timeout(default_start_timeout())
    }

    pub fn start_with_timeout(mut self, timeout: Duration) -> Self {
        let daemon = DaemonHandle::start(&self.config_dir, &self.socket_path);
        self.daemon = Some(daemon);
        self.wait_until_ready(timeout)
            .unwrap_or_else(|e| panic!("daemon did not become ready within {timeout:?}: {e}"));
        self
    }

    fn wait_until_ready(&self, timeout: Duration) -> Result<DaemonStatus, String> {
        let client = StatusClient::new(&self.socket_path);
        let deadline = Instant::now() + timeout;
        loop {
            let last_err = match client.status() {
                Ok(status) if status.ready => return Ok(status),
                Ok(status) => format!("daemon not ready yet: {status:?}"),
                Err(e) => e,
            };
            if Instant::now() >= deadline {
                let logs = self
                    .daemon
                    .as_ref()
                    .map(|d| d.captured_logs().join("\n"))
                    .unwrap_or_default();
                return Err(format!(
                    "{last_err}\n--- daemon logs ---\n{logs}\n--- end daemon logs ---"
                ));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    pub fn status(&self) -> Result<DaemonStatus, String> {
        StatusClient::new(&self.socket_path).status()
    }

    pub fn require_status(&self) -> DaemonStatus {
        self.status()
            .unwrap_or_else(|e| panic!("failed to get daemon status: {e}"))
    }

    pub fn require_list(&self) -> ProcessList {
        ProcessList(
            self.list_processes()
                .unwrap_or_else(|e| panic!("failed to list processes: {e}")),
        )
    }

    pub fn list_processes(&self) -> Result<Vec<ProcessSnapshot>, String> {
        ListClient::new(&self.socket_path).list()
    }

    pub fn process(&self, name: &str) -> Result<ProcessSnapshot, String> {
        self.find_process(name)
    }

    fn find_process(&self, name: &str) -> Result<ProcessSnapshot, String> {
        self.list_processes()?
            .into_iter()
            .find(|p| p.name == name)
            .ok_or_else(|| format!("process '{name}' not found"))
    }

    /// Poll until `name` is Running with a live PID (default timeout, no stability wait).
    pub fn wait_for_process_running(&self, name: &str) -> Result<ProcessSnapshot, String> {
        self.wait_for_process_running_with_stable(
            name,
            default_process_wait_timeout(),
            Duration::ZERO,
        )
    }

    /// Like [`wait_for_process_running`](Self::wait_for_process_running), but requires the
    /// process to stay Running for `stable_for` before returning.
    pub fn wait_for_process_running_stable(
        &self,
        name: &str,
        stable_for: Duration,
    ) -> Result<ProcessSnapshot, String> {
        self.wait_for_process_running_with_stable(name, default_process_wait_timeout(), stable_for)
    }

    pub fn wait_for_process_state(
        &self,
        name: &str,
        expected: ProcessExpect,
        timeout: Duration,
    ) -> Result<ProcessSnapshot, String> {
        if expected == ProcessExpect::Running {
            return self.wait_for_process_running_stable(name, stable_running_duration());
        }

        let client = ListClient::new(&self.socket_path);
        let deadline = Instant::now() + timeout;
        let expected_state = expected.as_str();
        loop {
            let last_err = match client.list() {
                Ok(processes) => match processes.iter().find(|p| p.name == name) {
                    Some(process) if process.state == expected_state => {
                        if process_matches_expect(process, expected) {
                            return Ok(process.clone());
                        }
                        format!(
                            "process '{name}' in state {expected_state} but PID expectation not met: {process:?}"
                        )
                    }
                    Some(process) => {
                        format!("process '{name}' not in {expected_state} yet: {process:?}")
                    }
                    None => format!("process '{name}' not in list: {processes:?}"),
                },
                Err(e) => e,
            };
            if Instant::now() >= deadline {
                return Err(self.format_wait_failure(last_err));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    pub fn assert_process_state_within(&self, name: &str, expected: ProcessExpect) {
        self.wait_for_process_state(name, expected, default_process_wait_timeout())
            .unwrap_or_else(|e| panic!("expected process '{name}' in state {expected:?}: {e}"));
    }

    pub fn start_process(&self, name: &str) -> Result<(), String> {
        let out = self.cli(&["start", name]);
        if !out.status.success() {
            return Err(format!(
                "start {name} failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        self.wait_for_process_state(name, ProcessExpect::Running, default_process_wait_timeout())?;
        Ok(())
    }

    pub fn stop_process(&self, name: &str) -> Result<(), String> {
        let out = self.cli(&["stop", name]);
        if !out.status.success() {
            return Err(format!(
                "stop {name} failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        self.wait_for_process_state(name, ProcessExpect::Stopped, default_process_wait_timeout())?;
        Ok(())
    }

    pub fn assert_start_process(&self, name: &str) {
        self.start_process(name)
            .unwrap_or_else(|e| panic!("expected start of '{name}' to succeed: {e}"));
    }

    pub fn assert_stop_process(&self, name: &str) {
        self.stop_process(name)
            .unwrap_or_else(|e| panic!("expected stop of '{name}' to succeed: {e}"));
    }

    fn reload(&self) -> Result<ReloadSnapshot, String> {
        let out = self.cli(&["reload", "--json"]);
        if !out.status.success() {
            return Err(format!(
                "reload --json failed (exit {:?})\nstdout: {}\nstderr: {}",
                out.status.code(),
                out.stdout,
                out.stderr,
            ));
        }
        serde_json::from_str(&out.stdout)
            .map_err(|e| format!("failed to parse reload JSON: {e}\nstdout: {}", out.stdout))
    }

    fn assert_reload(&self) -> ReloadSnapshot {
        self.reload()
            .unwrap_or_else(|e| panic!("expected reload to succeed: {e}"))
    }

    pub fn assert_reload_matches(&self, expected: ReloadExpect) {
        let pids_before: Vec<(&str, u64)> = expected
            .preserve_running_pids
            .iter()
            .map(|name| (name.as_str(), self.require_process_pid(name)))
            .collect();
        self.assert_reload().assert_matches(&expected);
        for (name, pid_before) in pids_before {
            self.wait_for_process_running(name)
                .unwrap_or_else(|e| panic!("expected process '{name}' running after reload: {e}"));
            assert_eq!(
                self.require_process_pid(name),
                pid_before,
                "unchanged reload should not respawn {name}"
            );
        }
    }

    fn require_process_pid(&self, name: &str) -> u64 {
        self.find_process(name)
            .unwrap_or_else(|e| panic!("{e}"))
            .pid
    }

    fn describe(&self, name_or_uuid: &str) -> Result<DescribeSnapshot, String> {
        DescribeClient::new(&self.socket_path).describe(name_or_uuid)
    }

    fn assert_describe(&self, name_or_uuid: &str) -> DescribeSnapshot {
        self.describe(name_or_uuid)
            .unwrap_or_else(|e| panic!("expected describe of '{name_or_uuid}' to succeed: {e}"))
    }

    pub fn assert_describe_matches(&self, name_or_uuid: &str, expected: DescribeExpect) {
        self.assert_describe(name_or_uuid).assert_matches(&expected);
    }

    fn format_wait_failure(&self, last_err: String) -> String {
        let logs = self
            .daemon
            .as_ref()
            .map(|d| d.captured_logs().join("\n"))
            .unwrap_or_default();
        format!("{last_err}\n--- daemon logs ---\n{logs}\n--- end daemon logs ---")
    }

    fn wait_for_process_running_with_stable(
        &self,
        name: &str,
        timeout: Duration,
        stable_for: Duration,
    ) -> Result<ProcessSnapshot, String> {
        let client = ListClient::new(&self.socket_path);
        let deadline = Instant::now() + timeout;
        let mut running_since: Option<Instant> = None;
        let mut stable_pid: Option<u64> = None;
        loop {
            let last_err = match client.list() {
                Ok(processes) => match processes.iter().find(|p| p.name == name) {
                    Some(process) if process.state == "Running" && process.pid > 0 => {
                        if !process_matches_expect(process, ProcessExpect::Running) {
                            running_since = None;
                            stable_pid = None;
                            format!(
                                "process '{name}' in Running but PID {} is not alive: {process:?}",
                                process.pid
                            )
                        } else if stable_for.is_zero() {
                            return Ok(process.clone());
                        } else {
                            let reset = stable_pid != Some(process.pid);
                            if reset || running_since.is_none() {
                                running_since = Some(Instant::now());
                                stable_pid = Some(process.pid);
                            }
                            let since = running_since.expect("running_since set above");
                            if since.elapsed() >= stable_for {
                                return Ok(process.clone());
                            }
                            format!(
                                "process '{name}' running (pid {}) but not stable for {stable_for:?} yet ({:?} elapsed)",
                                process.pid,
                                since.elapsed()
                            )
                        }
                    }
                    Some(process) => {
                        running_since = None;
                        stable_pid = None;
                        format!("process '{name}' not running yet: {process:?}")
                    }
                    None => {
                        running_since = None;
                        stable_pid = None;
                        format!("process '{name}' not in list: {processes:?}")
                    }
                },
                Err(e) => {
                    running_since = None;
                    stable_pid = None;
                    e
                }
            };
            if Instant::now() >= deadline {
                return Err(self.format_wait_failure(last_err));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    pub fn assert_process_state(&self, name: &str, expected: ProcessExpect) {
        self.require_list().assert_process_state(name, expected);
    }

    pub fn assert_process_last_exit_code(&self, name: &str, code: i32) {
        self.require_list().assert_last_exit_code(name, code);
    }

    pub fn wait_for_restart_count_at_least(
        &self,
        name: &str,
        min: u64,
        timeout: Duration,
    ) -> Result<ProcessSnapshot, String> {
        let deadline = Instant::now() + timeout;
        loop {
            let last_err = match self.process(name) {
                Ok(snap) if snap.restart_count >= min => return Ok(snap),
                Ok(snap) => format!(
                    "process '{name}' restart_count={} (want >={min})",
                    snap.restart_count
                ),
                Err(e) => e,
            };
            if Instant::now() >= deadline {
                return Err(self.format_wait_failure(last_err));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    pub fn assert_restart_count_at_least(&self, name: &str, min: u64) {
        self.wait_for_restart_count_at_least(name, min, default_process_wait_timeout())
            .unwrap_or_else(|e| panic!("expected restart_count >={min} for '{name}': {e}"));
    }

    /// Wait until `restart_count` equals `count`, state matches `expected`, and both hold for
    /// `stable_for`. Fails immediately if `restart_count` exceeds `count` (e.g. burst limit
    /// did not trip). Use `stable_for` longer than the fixture `restart_sec` when count is
    /// incremented at schedule time before the pending restart completes.
    pub fn wait_for_restart_count_terminal(
        &self,
        name: &str,
        count: u64,
        expected: ProcessExpect,
        stable_for: Duration,
        timeout: Duration,
    ) -> Result<ProcessSnapshot, String> {
        let expected_state = expected.as_str();
        let deadline = Instant::now() + timeout;
        let mut stable_since: Option<Instant> = None;
        loop {
            let snap = self.process(name)?;
            if snap.restart_count > count {
                return Err(format!(
                    "process '{name}' restart_count={} exceeds terminal cap {count}: {snap:?}",
                    snap.restart_count
                ));
            }
            let matches = snap.state == expected_state
                && snap.restart_count == count
                && process_matches_expect(&snap, expected);
            if matches {
                let since = stable_since.get_or_insert_with(Instant::now);
                if since.elapsed() >= stable_for {
                    return Ok(snap);
                }
            } else {
                stable_since = None;
            }
            if Instant::now() >= deadline {
                return Err(self.format_wait_failure(format!(
                    "process '{name}' did not stay in {expected_state} with restart_count={count} for {stable_for:?}, last: {snap:?}"
                )));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }

    pub fn assert_daemon_log_line_contains(&self, patterns: &[&str]) {
        assert!(
            self.daemon()
                .wait_for_log_line_contains(patterns, DEFAULT_TIMEOUT),
            "expected a daemon log line containing all of {patterns:?}, got:\n{}",
            self.daemon().captured_logs().join("\n")
        );
    }

    pub fn assert_config_skip_logged(&self, name: &str) {
        let prefix = format!("[{name}] skipping config");
        self.assert_daemon_log_line_contains(&[&prefix]);
    }

    pub fn assert_condition_path_not_met_logged(&self, name: &str, path: &str) {
        let prefix = format!("[{name}] condition_path_exists not met");
        self.assert_daemon_log_line_contains(&[&prefix, path]);
    }

    pub fn assert_pid_gone(&self, pid: u64) {
        assert!(
            wait_for_pid_gone(pid as u32, DEFAULT_TIMEOUT),
            "PID {pid} should be gone"
        );
    }

    pub fn assert_process_pid_changed(&self, name: &str, old_pid: u64) {
        self.assert_pid_gone(old_pid);
        self.wait_for_process_running(name)
            .unwrap_or_else(|e| panic!("expected process '{name}' running with new pid: {e}"));
        let new_pid = self.require_process_pid(name);
        assert_ne!(old_pid, new_pid, "PID should change for {name}");
    }

    /// Create a sleep process via CLI with optional extra args
    /// (e.g. `["--no-auto-start"]`, `["--env", "K=V"]`).
    pub fn create_sleep(&self, name: &str, extra_args: &[&str]) -> CliOutput {
        let (cmd, args) = test_helpers::sleep_cmd(300);
        let mut cli_args: Vec<String> = vec![
            "create".into(),
            "--name".into(),
            name.into(),
            "--command".into(),
            cmd.into(),
        ];
        if !args.is_empty() {
            cli_args.push("--args".into());
            cli_args.extend(args);
        }
        cli_args.extend(extra_args.iter().map(|s| s.to_string()));
        let refs: Vec<&str> = cli_args.iter().map(String::as_str).collect();
        self.cli(&refs)
    }

    /// Run a CLI command against this environment's daemon.
    ///
    /// Prefer the typed `cli_*` helpers below for standard subcommands in contract
    /// tests. Use this for `create` and other multi-flag invocations.
    pub fn cli(&self, args: &[&str]) -> CliOutput {
        let runner = CliRunner::new(&self.socket_path);
        runner.run(args)
    }

    pub fn cli_config(&self) -> CliOutput {
        self.cli(&["config"])
    }

    pub fn cli_config_json(&self) -> CliOutput {
        self.cli(&["config", "--json"])
    }

    pub fn cli_status(&self) -> CliOutput {
        self.cli(&["status"])
    }

    pub fn cli_status_json(&self) -> CliOutput {
        self.cli(&["status", "--json"])
    }

    pub fn cli_list(&self) -> CliOutput {
        self.cli(&["list"])
    }

    pub fn cli_list_json(&self) -> CliOutput {
        self.cli(&["list", "--json"])
    }

    pub fn cli_describe(&self, name: &str) -> CliOutput {
        self.cli(&["describe", name])
    }

    pub fn cli_describe_json(&self, name: &str) -> CliOutput {
        self.cli(&["describe", "--json", name])
    }

    pub fn cli_start(&self, name: &str) -> CliOutput {
        self.cli(&["start", name])
    }

    pub fn cli_start_json(&self, name: &str) -> CliOutput {
        self.cli(&["start", "--json", name])
    }

    pub fn cli_stop(&self, name: &str) -> CliOutput {
        self.cli(&["stop", name])
    }

    pub fn cli_stop_json(&self, name: &str) -> CliOutput {
        self.cli(&["stop", "--json", name])
    }

    pub fn cli_reload(&self) -> CliOutput {
        self.cli(&["reload"])
    }

    pub fn cli_reload_json(&self) -> CliOutput {
        self.cli(&["reload", "--json"])
    }

    /// Access the daemon handle for log inspection, PID checks, etc.
    pub fn daemon(&self) -> &DaemonHandle {
        self.daemon.as_ref().expect("daemon not started")
    }

    /// Get the daemon's own PID.
    pub fn daemon_pid(&self) -> u32 {
        self.daemon().pid()
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

fn assert_status_field(field: &str, actual: u32, expected: Option<u32>, status: &DaemonStatus) {
    if let Some(expected) = expected {
        assert_eq!(
            actual, expected,
            "status {field}: expected {expected}, got {actual}\nfull status: {status:?}"
        );
    }
}

fn process_matches_expect(process: &ProcessSnapshot, expected: ProcessExpect) -> bool {
    match expected {
        ProcessExpect::Created
        | ProcessExpect::Stopped
        | ProcessExpect::Failed
        | ProcessExpect::Exited => process.pid == 0,
        ProcessExpect::Running => {
            let pid = process.pid as u32;
            pid > 0 && pid_is_alive(pid)
        }
    }
}

fn spawn_log_reader<R: std::io::Read + Send + 'static>(
    stream: R,
    tag: &str,
    lines: Arc<Mutex<Vec<String>>>,
) -> std::thread::JoinHandle<()> {
    let tag = tag.to_string();
    std::thread::spawn(move || {
        let reader = BufReader::new(stream);
        for line in reader.lines() {
            match line {
                Ok(l) => {
                    eprintln!("[{tag}] {l}");
                    lines.lock().unwrap().push(l);
                }
                Err(_) => break,
            }
        }
    })
}

use dd_procmgrd::test_helpers;

fn fixtures_root() -> PathBuf {
    if let Some(marker) = option_env!("PROCMGR_TEST_FIXTURES_MARKER") {
        let marker = PathBuf::from(marker);
        return marker
            .parent()
            .unwrap_or_else(|| {
                panic!(
                    "invalid PROCMGR_TEST_FIXTURES_MARKER (expected .../fixtures/.marker): {}",
                    marker.display()
                )
            })
            .to_path_buf();
    }
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures")
}

fn install_process_fixture(env_root: &Path, config_dir: &Path, name: &str) {
    install_process_fixture_as(env_root, config_dir, name, name);
}

fn install_process_fixture_as(
    env_root: &Path,
    config_dir: &Path,
    fixture: &str,
    process_name: &str,
) {
    let bundle = fixtures_root().join(fixture);
    let yaml_src = bundle.join(format!("{fixture}.yaml"));
    assert!(
        yaml_src.is_file(),
        "fixture yaml not found: {}",
        yaml_src.display()
    );

    let scripts_dir = env_root.join("scripts");
    std::fs::create_dir_all(&scripts_dir)
        .unwrap_or_else(|e| panic!("failed to create {}: {e}", scripts_dir.display()));

    let script_path = scripts_dir.join(format!("{fixture}.py"));
    let py_src = bundle.join(format!("{fixture}.py"));
    if py_src.is_file() {
        std::fs::copy(&py_src, &script_path).unwrap_or_else(|e| {
            panic!(
                "failed to copy {} -> {}: {e}",
                py_src.display(),
                script_path.display()
            )
        });
    }

    let template = std::fs::read_to_string(&yaml_src)
        .unwrap_or_else(|e| panic!("failed to read fixture yaml {}: {e}", yaml_src.display()));
    let rendered = render_fixture_yaml(
        &template,
        fixture,
        env_root,
        config_dir,
        py_src.is_file().then_some(script_path.as_path()),
    );
    write_config(config_dir, process_name, &rendered);
}

fn render_fixture_yaml(
    template: &str,
    name: &str,
    env_root: &Path,
    config_dir: &Path,
    script_path: Option<&Path>,
) -> String {
    let python = test_helpers::python_exe();
    let root = env_root.display().to_string();
    let config_dir = config_dir.display().to_string();
    let script = script_path
        .map(|p| p.display().to_string())
        .unwrap_or_default();

    let mut rendered = template.to_string();
    rendered = rendered.replace(&format!("{{{{script:{name}}}}}"), &script);
    rendered = rendered.replace("{{script}}", &script);
    rendered = rendered.replace("{{python}}", &python);
    rendered = rendered.replace("{{root}}", &root);
    rendered = rendered.replace("{{config_dir}}", &config_dir);
    rendered
}

/// Write a YAML config file into `dir` with the given process `name`.
pub fn write_config(dir: &Path, name: &str, yaml: &str) {
    let path = dir.join(format!("{name}.yaml"));
    std::fs::write(&path, yaml)
        .unwrap_or_else(|e| panic!("failed to write {}: {e}", path.display()));
}

/// Check if a PID is still alive.
#[cfg(unix)]
pub fn pid_is_alive(pid: u32) -> bool {
    signal::kill(Pid::from_raw(pid as i32), None).is_ok()
}

/// Uses `WaitForSingleObject` with a zero timeout instead of
/// `GetExitCodeProcess` to avoid false positives when a process
/// exits with code 259 (`STILL_ACTIVE`).
#[cfg(windows)]
pub fn pid_is_alive(pid: u32) -> bool {
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::System::Threading::{
        OpenProcess, PROCESS_SYNCHRONIZE, WaitForSingleObject,
    };
    const WAIT_TIMEOUT: u32 = 258;
    unsafe {
        let handle = OpenProcess(PROCESS_SYNCHRONIZE, 0, pid);
        if handle.is_null() {
            return false;
        }
        let ret = WaitForSingleObject(handle, 0);
        CloseHandle(handle);
        ret == WAIT_TIMEOUT
    }
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

/// Force-kill a PID (simulates external crash, not dd-procmgr stop).
#[cfg(unix)]
pub fn kill_pid_force(pid: u32) {
    signal::kill(Pid::from_raw(pid as i32), Signal::SIGKILL)
        .unwrap_or_else(|e| panic!("failed to SIGKILL pid {pid}: {e}"));
}

/// Force-kill a PID (simulates external crash, not dd-procmgr stop).
#[cfg(windows)]
pub fn kill_pid_force(pid: u32) {
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_TERMINATE, TerminateProcess};

    unsafe {
        let handle = OpenProcess(PROCESS_TERMINATE, 0, pid);
        assert!(!handle.is_null(), "OpenProcess failed for pid {pid}");
        let ok = TerminateProcess(handle, 1);
        CloseHandle(handle);
        assert_ne!(ok, 0, "TerminateProcess failed for pid {pid}");
    }
}
