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
#[cfg(windows)]
pub fn sleep_cmd(secs: u32) -> (&'static str, Vec<String>) {
    (
        "cmd.exe",
        vec!["/C".into(), format!("timeout /t {secs} /nobreak >nul")],
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
#[cfg(windows)]
pub fn sleep_config_yaml() -> &'static str {
    "command: cmd.exe\nargs:\n  - '/C'\n  - 'timeout /t 300 /nobreak >nul'\n"
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
#[cfg(windows)]
pub fn pid_is_alive(pid: u32) -> bool {
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::System::Threading::{
        GetExitCodeProcess, OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION,
    };
    const STILL_ACTIVE: u32 = 259;
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if handle.is_null() {
            return false;
        }
        let mut exit_code: u32 = 0;
        let ok = GetExitCodeProcess(handle, &mut exit_code);
        CloseHandle(handle);
        ok != 0 && exit_code == STILL_ACTIVE
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

/// Command that spawns a grandchild (long sleep) whose PID is written to
/// `pid_file`, then sleeps forever while ignoring graceful-stop signals.
#[cfg(windows)]
pub fn grandchild_cmd(pid_file: &str) -> (&'static str, Vec<String>) {
    (
        "powershell.exe",
        vec![
            "-Command".into(),
            format!(
                "$p = Start-Process -PassThru -NoNewWindow powershell \
                 '-Command','Start-Sleep 3600'; \
                 $p.Id | Out-File -Encoding ascii '{pid_file}'; \
                 while($true){{Start-Sleep 60}}"
            ),
        ],
    )
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
