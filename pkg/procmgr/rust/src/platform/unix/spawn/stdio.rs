// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use log::warn;
use std::path::Path;
use std::process::Stdio;

use crate::spawn::StdioSetting;

/// Resolve portable stdio settings for `tokio::process::Command`.
pub(crate) fn to_command_stdio(setting: &StdioSetting, inheritable: bool) -> Stdio {
    let _ = inheritable;
    match setting {
        StdioSetting::Null => Stdio::null(),
        StdioSetting::Inherit => Stdio::inherit(),
        StdioSetting::File(path) => file_to_stdio(path),
    }
}

fn file_to_stdio(path: &Path) -> Stdio {
    match std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
    {
        Ok(f) => f.into(),
        Err(e) => {
            warn!(
                "failed to open stdio file {}: {e}, falling back to inherit",
                path.display()
            );
            Stdio::inherit()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_helpers;
    use std::path::PathBuf;

    fn command_stdio(yaml: &str) -> Stdio {
        let setting = match yaml {
            "null" => StdioSetting::Null,
            "inherit" | "" => StdioSetting::Inherit,
            path => StdioSetting::File(path.into()),
        };
        to_command_stdio(&setting, true)
    }

    #[test]
    fn inherit_spawns() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(command_stdio("inherit"))
            .stderr(command_stdio("inherit"))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn empty_string_matches_inherit() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(command_stdio(""))
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
            .stdout(command_stdio("null"))
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
            .stdout(command_stdio(path_str))
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
                .stdout(command_stdio(path_str))
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
        let bad_path = "/nonexistent_dir_pmgr_stdio/out.log";
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fallback_ok")
            .stdout(command_stdio(bad_path))
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "spawn with unopenable stdout path should still succeed (falls back to inherit)"
        );
    }
}
