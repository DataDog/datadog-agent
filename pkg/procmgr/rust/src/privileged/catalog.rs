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
            expected_command: r"C:\Windows\System32\cmd.exe",
            expected_args: &["/C", "echo", "procmgr-privileged-ok"],
            allowed_env: &[],
        },
        PrivilegedCommandSpec {
            expected_command: r"C:\Windows\System32\cmd.exe",
            expected_args: &["/C", "whoami"],
            allowed_env: &[],
        },
    ]
}

/// Reject requests that do not exactly match a catalog entry.
///
/// Returns the canonical catalog executable path to use at spawn time so callers
/// cannot substitute a same-basename binary from another directory.
pub fn validate<'a>(
    command: &str,
    args: &[String],
    env: &HashMap<String, String>,
) -> Result<&'a str> {
    let norm_cmd = normalize_path(command);
    let caller_basename = command_basename(command);
    let norm_args: Vec<String> = args.iter().map(|a| normalize_path(a)).collect();

    for spec in catalog() {
        let expected_cmd = normalize_path(spec.expected_command);
        if caller_basename != command_basename(spec.expected_command) {
            continue;
        }

        if norm_cmd.contains('\\') && norm_cmd != expected_cmd {
            bail!(
                "refusing privileged command: command path does not match catalog (got {command})"
            );
        }

        let expected_args: Vec<String> = spec
            .expected_args
            .iter()
            .map(|a| normalize_path(a))
            .collect();
        if norm_args != expected_args {
            continue;
        }

        for key in env.keys() {
            if !spec.allowed_env.contains(&key.as_str()) {
                bail!("refusing privileged command: disallowed env var {key}");
            }
        }

        return Ok(spec.expected_command);
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
        assert!(err.contains("not in catalog"), "{err}");
    }

    #[test]
    fn catalog_accepts_whoami_entry() {
        let cmd = validate(
            "cmd.exe",
            &["/C".into(), "whoami".into()],
            &HashMap::new(),
        )
        .unwrap();
        assert_eq!(cmd, r"C:\Windows\System32\cmd.exe");
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
    fn catalog_accepts_canonical_system32_path() {
        let cmd = validate(
            r"C:\Windows\System32\cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .unwrap();
        assert_eq!(cmd, r"C:\Windows\System32\cmd.exe");
    }

    #[test]
    fn catalog_substitutes_basename_with_canonical_path() {
        let cmd = validate(
            "cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .unwrap();
        assert_eq!(cmd, r"C:\Windows\System32\cmd.exe");
    }

    #[test]
    fn catalog_rejects_wrong_path_same_basename() {
        let err = validate(
            r"C:\Temp\cmd.exe",
            &["/C".into(), "echo".into(), "procmgr-privileged-ok".into()],
            &HashMap::new(),
        )
        .unwrap_err()
        .to_string();
        assert!(err.contains("path does not match catalog"), "{err}");
    }
}
