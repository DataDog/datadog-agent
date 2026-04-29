// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Cross-platform command helpers for unit and integration tests.
//!
//! Every test that spawns a child process should use these helpers instead of
//! hard-coding `/bin/sh`, `/bin/true`, etc., so the same test source compiles
//! (and, where possible, runs) on both Unix and Windows.

/// Command that succeeds immediately (exit 0).
#[cfg(unix)]
pub fn true_cmd() -> (&'static str, Vec<String>) {
    ("/usr/bin/true", vec![])
}

/// Command that succeeds immediately (exit 0).
#[cfg(windows)]
pub fn true_cmd() -> (&'static str, Vec<String>) {
    ("cmd.exe", vec!["/C".into(), "exit 0".into()])
}

/// Command that fails immediately (exit 1).
#[cfg(unix)]
pub fn false_cmd() -> (&'static str, Vec<String>) {
    ("/usr/bin/false", vec![])
}

/// Command that fails immediately (exit 1).
#[cfg(windows)]
pub fn false_cmd() -> (&'static str, Vec<String>) {
    ("cmd.exe", vec!["/C".into(), "exit 1".into()])
}

/// Command + args for sleeping `secs` seconds.
#[cfg(unix)]
pub fn sleep_cmd(secs: u32) -> (&'static str, Vec<String>) {
    ("/bin/sleep", vec![secs.to_string()])
}

/// Command + args for sleeping `secs` seconds.
/// Uses `ping -n` instead of `timeout` because `timeout.exe` is absent in
/// minimal Windows CI containers.
#[cfg(windows)]
pub fn sleep_cmd(secs: u32) -> (&'static str, Vec<String>) {
    (
        "ping.exe",
        vec!["-n".into(), (secs + 1).to_string(), "127.0.0.1".into()],
    )
}

/// Shell command + flag for running an inline script.
/// Usage: `let (sh, flag) = shell_cmd(); Command::new(sh).args([flag, "exit 42"])`
#[cfg(unix)]
pub fn shell_cmd() -> (&'static str, &'static str) {
    ("/bin/sh", "-c")
}

/// Shell command + flag for running an inline script.
#[cfg(windows)]
pub fn shell_cmd() -> (&'static str, &'static str) {
    ("cmd.exe", "/C")
}

/// Build an `ExitStatus` with the given exit code.
pub fn exit_status(code: i32) -> std::process::ExitStatus {
    let (sh, flag) = shell_cmd();
    std::process::Command::new(sh)
        .args([flag, &format!("exit {code}")])
        .status()
        .unwrap()
}

/// YAML config snippet for a process that sleeps for a long time.
#[cfg(unix)]
pub fn sleep_config_yaml() -> &'static str {
    "command: /bin/sleep\nargs:\n  - '300'\n"
}

/// YAML config snippet for a process that sleeps for a long time.
/// Uses `ping -n` instead of `timeout` because `timeout.exe` is absent in
/// minimal Windows CI containers.
#[cfg(windows)]
pub fn sleep_config_yaml() -> &'static str {
    "command: ping.exe\nargs:\n  - '-n'\n  - '301'\n  - '127.0.0.1'\n"
}

/// YAML for a process that exits successfully and never restarts.
#[cfg(unix)]
pub fn true_config_yaml() -> &'static str {
    "command: /usr/bin/true\nrestart: never\n"
}

/// YAML for a process that exits successfully and never restarts.
#[cfg(windows)]
pub fn true_config_yaml() -> &'static str {
    "command: cmd.exe\nargs:\n  - '/C'\n  - 'exit 0'\nrestart: never\n"
}

/// YAML for a process that exits with failure and never restarts.
#[cfg(unix)]
pub fn false_config_yaml() -> &'static str {
    "command: /usr/bin/false\nrestart: never\n"
}

/// YAML for a process that exits with failure and never restarts.
#[cfg(windows)]
pub fn false_config_yaml() -> &'static str {
    "command: cmd.exe\nargs:\n  - '/C'\n  - 'exit 1'\nrestart: never\n"
}

/// Command that ignores graceful-stop and sleeps forever.
/// Used to test forced-kill (SIGKILL / TerminateProcess) on timeout.
///
/// The loop ensures that even though `sleep` (a child) is killed by SIGTERM,
/// the shell (which traps SIGTERM) restarts it, keeping the process alive
/// until SIGKILL arrives.
#[cfg(unix)]
pub fn trap_term_sleep() -> (&'static str, Vec<String>) {
    (
        "/bin/sh",
        vec![
            "-c".into(),
            "trap '' TERM; while true; do sleep 60; done".into(),
        ],
    )
}

/// Command that ignores graceful-stop and sleeps forever.
/// Used to test forced-kill (TerminateProcess) on timeout.
///
/// PowerShell ignores CTRL_BREAK_EVENT by default, so the process
/// outlives any stop_timeout and forces escalation to TerminateProcess.
#[cfg(windows)]
pub fn trap_term_sleep() -> (&'static str, Vec<String>) {
    (
        "powershell.exe",
        vec!["-Command".into(), "while($true){Start-Sleep 60}".into()],
    )
}

/// Shell command that exits with the value of the given environment variable.
#[cfg(unix)]
pub fn exit_env_cmd(var: &str) -> (&'static str, Vec<String>) {
    let (sh, flag) = shell_cmd();
    (sh, vec![flag.to_string(), format!("exit ${var}")])
}

/// Shell command that exits with the value of the given environment variable.
#[cfg(windows)]
pub fn exit_env_cmd(var: &str) -> (&'static str, Vec<String>) {
    let (sh, flag) = shell_cmd();
    (sh, vec![flag.to_string(), format!("exit %{var}%")])
}

/// Shell command that exits with the given code.
pub fn exit_cmd(code: i32) -> (&'static str, Vec<String>) {
    let (sh, flag) = shell_cmd();
    (sh, vec![flag.to_string(), format!("exit {code}")])
}

// ---------------------------------------------------------------------------
// YAML config builders
// ---------------------------------------------------------------------------

/// Build a YAML config from a command, args, and extra options.
pub fn cmd_yaml(cmd: &str, args: &[String], extra: &str) -> String {
    let mut yaml = format!("command: {cmd}\n");
    if !args.is_empty() {
        yaml.push_str("args:\n");
        for arg in args {
            yaml.push_str(&format!("  - '{}'\n", arg));
        }
    }
    yaml.push_str(extra);
    yaml
}

/// Sleep config with extra options appended.
pub fn sleep_config_with(extra: &str) -> String {
    let (cmd, args) = sleep_cmd(300);
    cmd_yaml(cmd, &args, extra)
}

/// True-command config with extra options (NO default restart policy).
pub fn true_config_with(extra: &str) -> String {
    let (cmd, args) = true_cmd();
    cmd_yaml(cmd, &args, extra)
}

/// False-command config with extra options (NO default restart policy).
pub fn false_config_with(extra: &str) -> String {
    let (cmd, args) = false_cmd();
    cmd_yaml(cmd, &args, extra)
}

/// The args for the platform's sleep command as a JSON value (for assertions).
pub fn sleep_args_json() -> serde_json::Value {
    let (_, args) = sleep_cmd(300);
    serde_json::json!(args)
}

/// The args for the platform's sleep command joined for display (for assertions).
pub fn sleep_args_display() -> String {
    let (_, args) = sleep_cmd(300);
    args.join(" ")
}

/// Cross-platform temp directory path string.
pub fn temp_dir_str() -> String {
    std::env::temp_dir().display().to_string()
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

/// Fixed UUID for deterministic tests.
pub fn test_uuid() -> String {
    "00000000-0000-0000-0000-000000000000".to_string()
}

/// Best-effort teardown: force-kill a process group so tests don't leak children.
pub fn cleanup_process(pid: u32) {
    let _ = crate::platform::send_force_kill(pid);
}

/// Check whether a process is still alive.
#[cfg(unix)]
pub fn pid_is_alive(pid: u32) -> bool {
    // signal 0 = check existence without sending a real signal.
    nix::sys::signal::kill(nix::unistd::Pid::from_raw(pid as i32), None).is_ok()
}

/// Check whether a process is still alive.
///
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

/// RAII handle to a Windows process. Holding this keeps the kernel object
/// alive, so `is_alive()` is immune to PID reuse.
#[cfg(windows)]
pub struct ProcessHandle(windows_sys::Win32::Foundation::HANDLE);

#[cfg(windows)]
impl ProcessHandle {
    /// Open a handle to the given PID. Returns `None` if the process
    /// doesn't exist or access is denied.
    pub fn open(pid: u32) -> Option<Self> {
        use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_SYNCHRONIZE};
        let handle = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, 0, pid) };
        if handle.is_null() {
            None
        } else {
            Some(Self(handle))
        }
    }

    /// Check whether the process is still running.
    pub fn is_alive(&self) -> bool {
        const WAIT_TIMEOUT: u32 = 258;
        let ret = unsafe { windows_sys::Win32::System::Threading::WaitForSingleObject(self.0, 0) };
        ret == WAIT_TIMEOUT
    }
}

#[cfg(windows)]
impl Drop for ProcessHandle {
    fn drop(&mut self) {
        unsafe { windows_sys::Win32::Foundation::CloseHandle(self.0) };
    }
}

/// Command that spawns a grandchild (long sleep) whose PID is written to
/// `pid_file`, then sleeps forever while ignoring graceful-stop signals.
/// Used to verify that force-kill (Job Object / SIGKILL to pgid) cleans
/// up the entire descendant tree.
#[cfg(unix)]
pub fn grandchild_cmd(pid_file: &str) -> (&'static str, Vec<String>) {
    (
        "/bin/sh",
        vec![
            "-c".into(),
            format!("trap '' TERM; /bin/sleep 3600 & echo $! > {pid_file}; wait"),
        ],
    )
}

/// Set up a grandchild test: creates a temp dir, builds a `ProcessConfig`
/// that spawns a grandchild writing its PID to a file, and returns both.
/// The caller must keep the `TempDir` alive for the duration of the test.
///
/// Only used on Unix — the Windows descendant-cleanup test queries the Job
/// Object for descendant PIDs instead of relying on a PID file.
#[cfg(all(test, unix))]
pub fn grandchild_config(
    stop_timeout: u64,
) -> (
    tempfile::TempDir,
    std::path::PathBuf,
    crate::config::ProcessConfig,
) {
    let dir = tempfile::tempdir().unwrap();
    let pid_file = dir.path().join("grandchild.pid");
    let pid_file_str = pid_file.to_str().unwrap();
    let (cmd, args) = grandchild_cmd(pid_file_str);
    let mut cfg = make_config(cmd, args);
    cfg.stop_timeout = Some(stop_timeout);
    (dir, pid_file, cfg)
}

/// Poll a PID file until it contains a valid PID, or panic after `timeout`.
pub async fn wait_for_pid_file(path: &std::path::Path, timeout: std::time::Duration) -> u32 {
    let deadline = tokio::time::Instant::now() + timeout;
    loop {
        if let Ok(contents) = std::fs::read_to_string(path)
            && let Ok(pid) = contents.trim().parse::<u32>()
        {
            return pid;
        }
        assert!(
            tokio::time::Instant::now() < deadline,
            "timed out waiting for PID file: {}",
            path.display()
        );
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    }
}

/// Build a `ProcessConfig` with null stdio, suitable for tests.
pub fn make_config(command: &str, args: Vec<String>) -> crate::config::ProcessConfig {
    crate::config::ProcessConfig {
        command: command.to_string(),
        args,
        stdout: "null".to_string(),
        stderr: "null".to_string(),
        ..Default::default()
    }
}
