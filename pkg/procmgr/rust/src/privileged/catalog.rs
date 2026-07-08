// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::collections::HashMap;

/// Host-side allowlist entry for [`super::validate`].
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PrivilegedCommandSpec {
    pub expected_command: &'static str,
    pub expected_args: &'static [&'static str],
    pub allowed_env: &'static [&'static str],
}

/// Embedded catalog of commands PAR may request via RunPrivilegedCommand.
fn catalog() -> &'static [PrivilegedCommandSpec] {
    &[
        PrivilegedCommandSpec {
            expected_command: "cmd.exe",
            expected_args: &["/C", "echo", "procmgr-privileged-ok"],
            allowed_env: &[],
        },
        PrivilegedCommandSpec {
            expected_command: "cmd.exe",
            expected_args: &["/C", "whoami"],
            allowed_env: &[],
        },
    ]
}

/// Reject requests that do not exactly match a catalog entry.
pub fn validate(command: &str, args: &[String], env: &HashMap<String, String>) -> Result<()> {
    let norm_cmd = command_basename(command);
    let norm_args: Vec<String> = args.iter().map(|a| normalize_path(a)).collect();

    for spec in catalog() {
        if norm_cmd != command_basename(spec.expected_command) {
            continue;
        }

        let expected_args: Vec<String> = spec
            .expected_args
            .iter()
            .map(|a| normalize_path(a))
            .collect();
        if norm_args != expected_args {
            bail!(
                "refusing privileged command: unexpected args {args:?} (expected {:?})",
                spec.expected_args
            );
        }

        for key in env.keys() {
            if !spec.allowed_env.contains(&key.as_str()) {
                bail!("refusing privileged command: disallowed env var {key}");
            }
        }

        return Ok(());
    }

    bail!("refusing privileged command: command not in catalog (got {command})");
}

fn normalize_path(s: &str) -> String {
    s.replace('/', "\\").to_ascii_lowercase()
}

fn command_basename(s: &str) -> String {
    let normalized = normalize_path(s);
    normalized
        .rsplit('\\')
        .next()
        .unwrap_or(normalized.as_str())
        .to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn catalog_rejects_unknown_command() {
        let err = validate("powershell.exe", &["-Command".into()], &HashMap::new())
            .unwrap_err()
            .to_string();
        assert!(err.contains("not in catalog"), "{err}");
    }

    #[test]
    fn catalog_rejects_wrong_args() {
        let err = validate(
            "cmd.exe",
            &["/C".into(), "exit".into(), "1".into()],
            &HashMap::new(),
        )
        .unwrap_err()
        .to_string();
        assert!(err.contains("unexpected args"), "{err}");
    }

    #[test]
    fn catalog_rejects_disallowed_env() {
        let mut env = HashMap::new();
        env.insert("EVIL".into(), "1".into());
        let err = validate(
            "cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &env,
        )
        .unwrap_err()
        .to_string();
        assert!(err.contains("disallowed env"), "{err}");
    }

    #[test]
    fn catalog_accepts_test_entry() {
        validate(
            "cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .unwrap();
    }

    #[test]
    fn catalog_accepts_normalized_paths() {
        validate(
            r"C:\Windows\System32\cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .unwrap();
    }
}
