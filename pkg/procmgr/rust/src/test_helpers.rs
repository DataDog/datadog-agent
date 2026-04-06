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

/// Fixed UUID for deterministic tests.
pub fn test_uuid() -> String {
    "00000000-0000-0000-0000-000000000000".to_string()
}

/// Best-effort teardown: force-kill a process group so tests don't leak children.
pub fn cleanup_process(pid: u32) {
    let _ = crate::platform::send_force_kill(pid);
}

/// Build a `ProcessConfig` with null stdio, suitable for tests.
pub fn make_config<S: Into<String>>(command: &str, args: Vec<S>) -> crate::config::ProcessConfig {
    crate::config::ProcessConfig {
        command: command.to_string(),
        args: args.into_iter().map(Into::into).collect(),
        stdout: "null".to_string(),
        stderr: "null".to_string(),
        ..Default::default()
    }
}
