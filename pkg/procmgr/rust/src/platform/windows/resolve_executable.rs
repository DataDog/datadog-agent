// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve bare executable names through `PATH`/`PATHEXT` before ACL validation.
//!
//! Go's `exec.Command` calls `LookPath` for bare names before `filesystem.CheckRights`;
//! procmgr must match that so `secret_backend_command` values like `secret-generic-connector.exe`
//! resolve under the Agent service environment rather than failing as nonexistent paths.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use anyhow::{Result, bail};

const DEFAULT_PATHEXT: &str = ".COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC";

/// Resolve `command` the way `exec.Command` would under `env` (PATH + PATHEXT).
pub(crate) fn resolve_executable_in_env(
    command: &str,
    env: &HashMap<String, String>,
) -> Result<String> {
    if command.trim().is_empty() {
        bail!("secretBackendCommand is empty");
    }

    if contains_path_separator(command) {
        if path_is_existing_file(Path::new(command)) {
            return Ok(command.to_string());
        }
        bail!("secretBackendCommand '{command}' does not exist");
    }

    let path_dirs = path_directories(env);
    let extensions = pathext_extensions(env);

    if path_has_extension(command) {
        if let Some(resolved) = find_on_path(command, &path_dirs) {
            return Ok(resolved);
        }
        bail!("secretBackendCommand '{command}' does not exist");
    }

    for dir in &path_dirs {
        for ext in &extensions {
            let candidate_name = format!("{command}{ext}");
            let candidate = dir.join(&candidate_name);
            if path_is_existing_file(&candidate) {
                return Ok(candidate.to_string_lossy().into_owned());
            }
        }
    }

    bail!("secretBackendCommand '{command}' does not exist");
}

fn path_is_existing_file(path: &Path) -> bool {
    if path.is_file() {
        return true;
    }
    #[cfg(windows)]
    {
        let Some(name) = path.file_name() else {
            return false;
        };
        let Some(parent) = path.parent() else {
            return false;
        };
        let Ok(entries) = std::fs::read_dir(parent) else {
            return false;
        };
        return entries.flatten().any(|entry| {
            entry.file_name().eq_ignore_ascii_case(name)
                && entry.file_type().is_ok_and(|t| t.is_file())
        });
    }
    #[cfg(not(windows))]
    false
}

fn contains_path_separator(command: &str) -> bool {
    command.contains('\\') || command.contains('/')
}

fn path_has_extension(command: &str) -> bool {
    Path::new(command).extension().is_some()
}

fn env_var_case_insensitive<'a>(env: &'a HashMap<String, String>, key: &str) -> Option<&'a str> {
    env.iter()
        .find(|(name, _)| name.eq_ignore_ascii_case(key))
        .map(|(_, value)| value.as_str())
        .filter(|value| !value.is_empty())
}

fn path_directories(env: &HashMap<String, String>) -> Vec<PathBuf> {
    env_var_case_insensitive(env, "PATH")
        .map(|path| {
            path.split(';')
                .map(str::trim)
                .filter(|entry| !entry.is_empty())
                .map(PathBuf::from)
                .collect()
        })
        .unwrap_or_default()
}

fn pathext_extensions(env: &HashMap<String, String>) -> Vec<String> {
    let raw = env_var_case_insensitive(env, "PATHEXT").unwrap_or(DEFAULT_PATHEXT);
    raw.split(';')
        .map(str::trim)
        .filter(|entry| !entry.is_empty())
        .map(|entry| {
            if entry.starts_with('.') {
                entry.to_string()
            } else {
                format!(".{entry}")
            }
        })
        .collect()
}

fn find_on_path(command: &str, path_dirs: &[PathBuf]) -> Option<String> {
    for dir in path_dirs {
        let candidate = dir.join(command);
        if path_is_existing_file(&candidate) {
            return Some(candidate.to_string_lossy().into_owned());
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::sync::atomic::{AtomicU64, Ordering};

    static NEXT_ID: AtomicU64 = AtomicU64::new(0);

    fn temp_bin_dir() -> (tempfile::TempDir, PathBuf) {
        let dir = tempfile::tempdir().expect("tempdir");
        let bin = dir
            .path()
            .join(format!("bin{}", NEXT_ID.fetch_add(1, Ordering::Relaxed)));
        fs::create_dir_all(&bin).expect("create bin dir");
        (dir, bin)
    }

    #[test]
    fn resolve_bare_name_through_path_and_pathext() {
        let (_dir, bin) = temp_bin_dir();
        let exe = bin.join("secret-backend.exe");
        fs::write(&exe, b"").expect("write exe");

        let env = HashMap::from([("Path".to_string(), bin.to_string_lossy().into_owned())]);
        let resolved =
            resolve_executable_in_env("secret-backend", &env).expect("resolve bare name");
        assert_eq!(resolved, exe.to_string_lossy());
    }

    #[test]
    fn resolve_bare_name_with_extension_on_path() {
        let (_dir, bin) = temp_bin_dir();
        let exe = bin.join("connector.exe");
        fs::write(&exe, b"").expect("write exe");

        let env = HashMap::from([("PATH".to_string(), bin.to_string_lossy().into_owned())]);
        let resolved =
            resolve_executable_in_env("connector.exe", &env).expect("resolve with extension");
        assert_eq!(resolved, exe.to_string_lossy());
    }

    #[test]
    fn resolve_explicit_path_without_path_search() {
        let dir = tempfile::tempdir().expect("tempdir");
        let exe = dir.path().join("nested.exe");
        fs::write(&exe, b"").expect("write exe");

        let env = HashMap::new();
        let path = exe.to_string_lossy().into_owned();
        let resolved = resolve_executable_in_env(&path, &env).expect("resolve explicit path");
        assert_eq!(resolved, path);
    }

    #[test]
    fn resolve_missing_bare_name_errors() {
        let env = HashMap::from([("PATH".to_string(), r"C:\missing".to_string())]);
        let err = resolve_executable_in_env("nowhere", &env).unwrap_err();
        assert!(
            err.to_string().contains("does not exist"),
            "unexpected error: {err:#}"
        );
    }

    #[test]
    fn resolve_missing_explicit_path_errors() {
        let env = HashMap::new();
        let err = resolve_executable_in_env(r"C:\missing\backend.exe", &env).unwrap_err();
        assert!(
            err.to_string().contains("does not exist"),
            "unexpected error: {err:#}"
        );
    }
}
