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
pub fn true_cmd() -> (&'static str, Vec<&'static str>) {
    ("/usr/bin/true", vec![])
}

#[cfg(windows)]
pub fn true_cmd() -> (&'static str, Vec<&'static str>) {
    ("cmd.exe", vec!["/C", "exit 0"])
}

/// Command that fails immediately (exit 1).
#[cfg(unix)]
pub fn false_cmd() -> (&'static str, Vec<&'static str>) {
    ("/usr/bin/false", vec![])
}

#[cfg(windows)]
pub fn false_cmd() -> (&'static str, Vec<&'static str>) {
    ("cmd.exe", vec!["/C", "exit 1"])
}

/// Command + args for sleeping `secs` seconds.
#[cfg(unix)]
pub fn sleep_cmd(secs: u32) -> (&'static str, Vec<String>) {
    ("/bin/sleep", vec![secs.to_string()])
}

#[cfg(windows)]
pub fn sleep_cmd(secs: u32) -> (&'static str, Vec<String>) {
    ("cmd.exe", vec!["/C".into(), format!("timeout /t {secs} /nobreak >nul")])
}

/// Shell command + flag for running an inline script.
/// Usage: `let (sh, flag) = shell_cmd(); Command::new(sh).args([flag, "exit 42"])`
#[cfg(unix)]
pub fn shell_cmd() -> (&'static str, &'static str) {
    ("/bin/sh", "-c")
}

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

#[cfg(windows)]
pub fn sleep_config_yaml() -> &'static str {
    "command: cmd.exe\nargs:\n  - '/C'\n  - 'timeout /t 300 /nobreak >nul'\n"
}

/// YAML for a process that exits successfully and never restarts.
pub fn true_config_yaml() -> &'static str {
    #[cfg(unix)]
    {
        "command: /usr/bin/true\nrestart: never\n"
    }
    #[cfg(windows)]
    {
        "command: cmd.exe\nargs:\n  - '/C'\n  - 'exit 0'\nrestart: never\n"
    }
}

/// YAML for a process that exits with failure and never restarts.
pub fn false_config_yaml() -> &'static str {
    #[cfg(unix)]
    {
        "command: /usr/bin/false\nrestart: never\n"
    }
    #[cfg(windows)]
    {
        "command: cmd.exe\nargs:\n  - '/C'\n  - 'exit 1'\nrestart: never\n"
    }
}

/// Command that ignores graceful-stop and sleeps forever.
/// Used to test forced-kill (SIGKILL / TerminateProcess) on timeout.
#[cfg(unix)]
pub fn trap_term_sleep() -> (&'static str, Vec<&'static str>) {
    ("/bin/sh", vec!["-c", "trap '' TERM; sleep 60"])
}

#[cfg(windows)]
pub fn trap_term_sleep() -> (&'static str, Vec<&'static str>) {
    // On Windows there is no SIGTERM to trap; the graceful stop sends
    // CTRL_BREAK_EVENT which cmd.exe ignores by default, so a plain
    // long sleep is equivalent.
    ("cmd.exe", vec!["/C", "timeout /t 60 /nobreak >nul"])
}

/// Shell command that exits with the value of the given environment variable.
#[cfg(unix)]
pub fn exit_env_cmd(var: &str) -> (&'static str, Vec<String>) {
    let (sh, flag) = shell_cmd();
    (sh, vec![flag.to_string(), format!("exit ${var}")])
}

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
