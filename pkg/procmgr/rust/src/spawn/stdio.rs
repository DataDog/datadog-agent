// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use log::warn;
use std::process::Stdio;

use crate::platform;

pub(crate) fn is_inherit_or_null(config: &str) -> bool {
    matches!(config, "inherit" | "" | "null")
}

/// Reject privileged stdio configs that would open file paths before catalog validation.
pub(super) fn require_inherit_or_null(
    process_name: &str,
    stdout: &str,
    stderr: &str,
) -> Result<()> {
    if !is_inherit_or_null(stdout) || !is_inherit_or_null(stderr) {
        bail!("[{process_name}] refusing privileged spawn: stdout/stderr must be inherit or null");
    }
    Ok(())
}

/// Map inherit/null stdio only. Caller must run [`require_inherit_or_null`] first.
pub(super) fn from_inherit_or_null(s: &str) -> Stdio {
    match s {
        "null" => Stdio::null(),
        "inherit" | "" => Stdio::inherit(),
        _ => Stdio::null(),
    }
}

pub(super) fn stdout_from_config(yaml_value: &str) -> Stdio {
    from_config(yaml_value, platform::stdout_inheritable())
}

pub(super) fn stderr_from_config(yaml_value: &str) -> Stdio {
    from_config(yaml_value, platform::stderr_inheritable())
}

fn from_config(yaml_value: &str, inheritable: bool) -> Stdio {
    #[cfg(not(windows))]
    let _ = inheritable;
    #[cfg(windows)]
    if !inheritable && matches!(yaml_value, "inherit" | "") {
        return Stdio::null();
    }
    from_str(yaml_value)
}

fn from_str(s: &str) -> Stdio {
    match s {
        "null" => Stdio::null(),
        "inherit" | "" => Stdio::inherit(),
        path => from_path(path),
    }
}

fn from_path(path: &str) -> Stdio {
    match std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
    {
        Ok(f) => f.into(),
        Err(e) => {
            warn!("failed to open stdio file {path}: {e}, falling back to inherit");
            Stdio::inherit()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_helpers;

    #[test]
    fn require_inherit_or_null_rejects_file_paths() {
        let err = require_inherit_or_null("proc", r"C:\logs\out.log", "inherit").unwrap_err();
        assert!(err.to_string().contains("inherit or null"));
    }

    #[test]
    fn inherit_spawns() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(from_str("inherit"))
            .stderr(from_str("inherit"))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn empty_string_matches_inherit() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(from_str(""))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn null_discards_child_stdout() {
        let (sh, flag) = test_helpers::shell_cmd();
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo hello")
            .stdout(from_str("null"))
            .output()
            .unwrap();
        assert!(
            out.stdout.is_empty(),
            "stdout should be discarded, got {:?}",
            String::from_utf8_lossy(&out.stdout)
        );
    }

    #[test]
    fn writable_path_redirect() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_redirect.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        let status = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fileline")
            .stdout(from_str(path_str))
            .status()
            .unwrap();
        assert!(status.success());
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(
            contents.contains("fileline"),
            "expected fileline in log, got {contents:?}"
        );
    }

    #[test]
    fn path_appends_on_respawn() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_append.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        for msg in ["first", "second"] {
            let status = std::process::Command::new(sh)
                .arg(flag)
                .arg(format!("echo {msg}"))
                .stdout(from_str(path_str))
                .status()
                .unwrap();
            assert!(status.success());
        }
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(contents.contains("first"), "got {contents:?}");
        assert!(contents.contains("second"), "got {contents:?}");
    }

    #[test]
    fn unopenable_path_falls_back_to_inherit() {
        let (sh, flag) = test_helpers::shell_cmd();
        #[cfg(unix)]
        let bad_path = "/nonexistent_dir_pmgr_stdio/out.log";
        #[cfg(windows)]
        let bad_path = r"C:\nonexistent_dir_pmgr_stdio\out.log";
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fallback_ok")
            .stdout(from_str(bad_path))
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "spawn with unopenable stdout path should still succeed (falls back to inherit)"
        );
        // Child stdout is inherited (not piped), so `out.stdout` is empty; success is the signal.
    }
}
