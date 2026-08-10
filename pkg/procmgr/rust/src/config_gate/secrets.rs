// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve `ENC[...]` config values via `secret_backend_command`.
//!
//! Mirrors `comp/core/secrets/utils.IsEnc` and the command-backend payload in
//! `comp/core/secrets/impl/fetch_secret.go`. Only `secret_backend_command` is
//! supported (not `secret_backend_type` / `multi_secret_backends`).
//!
//! Backend settings follow Agent config precedence: `DD_SECRET_BACKEND_*` env
//! vars (including the core Agent service SCM `Environment` on Windows) override
//! `datadog.yaml`. The backend command always runs as the core Agent service
//! account (`dd-agent` / `datadogagent`), not as the procmgr supervisor or a
//! Privileged managed-child identity.

use std::collections::HashMap;
use std::path::Path;
use std::sync::{Mutex, OnceLock};
use std::time::Duration;

use anyhow::{Context, Result, bail};
use log::debug;
use serde_json::Value;

use super::env_bindings;
use super::yaml_load;

const PAYLOAD_VERSION: &str = "1.1";
const DEFAULT_TIMEOUT_SECS: u64 = 30;
const DEFAULT_MAX_OUTPUT_BYTES: usize = 1_048_576;

#[derive(Clone)]
struct Backend {
    command: String,
    arguments: Vec<String>,
    timeout_secs: u64,
    max_output_bytes: usize,
    remove_trailing_line_break: bool,
}

static HANDLE_CACHE: OnceLock<Mutex<HashMap<String, String>>> = OnceLock::new();
static BACKEND_CACHE: OnceLock<Mutex<HashMap<String, Option<Backend>>>> = OnceLock::new();

pub(super) fn is_enc(value: &str) -> bool {
    enc_handle(value).is_some()
}

/// Resolve `ENC[handle]` through the Agent secret backend; return `raw` unchanged otherwise.
pub(super) fn resolve_config_string(raw: &str, agent_yaml: &str) -> String {
    let Some(handle) = enc_handle(raw) else {
        return raw.to_owned();
    };
    match resolve_handle(&handle, agent_yaml) {
        Ok(value) => value,
        Err(err) => {
            debug!("config gate secret resolve failed for {handle}: {err:#}");
            raw.to_owned()
        }
    }
}

fn enc_handle(value: &str) -> Option<String> {
    let trimmed = value.trim();
    let inner = trimmed.strip_prefix("ENC[")?.strip_suffix(']')?.trim();
    (!inner.is_empty()).then(|| inner.to_owned())
}

fn resolve_handle(handle: &str, agent_yaml: &str) -> Result<String> {
    if let Some(cached) = cached_handle(handle) {
        return Ok(cached);
    }
    let Some(backend) = backend_for(agent_yaml)? else {
        bail!("secret_backend_command is not configured");
    };
    let value = fetch_secret(&backend, handle)?;
    cache_handle(handle, &value);
    Ok(value)
}

fn cached_handle(handle: &str) -> Option<String> {
    HANDLE_CACHE
        .get_or_init(|| Mutex::new(HashMap::new()))
        .lock()
        .ok()?
        .get(handle)
        .cloned()
}

fn cache_handle(handle: &str, value: &str) {
    if let Ok(mut cache) = HANDLE_CACHE
        .get_or_init(|| Mutex::new(HashMap::new()))
        .lock()
    {
        cache.insert(handle.to_owned(), value.to_owned());
    }
}

/// Drop cached handles and backend settings so reload re-queries the secret backend.
pub(super) fn clear_caches() {
    if let Some(cache) = HANDLE_CACHE.get()
        && let Ok(mut guard) = cache.lock()
    {
        guard.clear();
    }
    if let Some(cache) = BACKEND_CACHE.get()
        && let Ok(mut guard) = cache.lock()
    {
        guard.clear();
    }
}

fn backend_for(agent_yaml: &str) -> Result<Option<Backend>> {
    let cache = BACKEND_CACHE.get_or_init(|| Mutex::new(HashMap::new()));
    let mut guard = cache.lock().expect("backend cache lock");
    if let Some(backend) = guard.get(agent_yaml) {
        return Ok(backend.clone());
    }
    let backend = load_backend(agent_yaml)?;
    guard.insert(agent_yaml.to_owned(), backend.clone());
    Ok(backend)
}

fn load_backend(agent_yaml: &str) -> Result<Option<Backend>> {
    let root = load_agent_yaml_root(agent_yaml)?;
    let command = env_string("DD_SECRET_BACKEND_COMMAND").or_else(|| {
        root.as_ref()
            .and_then(|yaml| yaml_string(yaml, "secret_backend_command"))
    });
    let Some(command) = command else {
        return Ok(None);
    };
    if command.trim().is_empty() {
        return Ok(None);
    }
    let arguments = env_string_list("DD_SECRET_BACKEND_ARGUMENTS")
        .or_else(|| {
            root.as_ref()
                .map(|yaml| yaml_string_list(yaml, "secret_backend_arguments"))
        })
        .unwrap_or_default();
    let timeout_secs = env_u64("DD_SECRET_BACKEND_TIMEOUT")
        .or_else(|| {
            root.as_ref()
                .and_then(|yaml| yaml_u64(yaml, "secret_backend_timeout"))
        })
        .unwrap_or(DEFAULT_TIMEOUT_SECS);
    let max_output_bytes = env_usize("DD_SECRET_BACKEND_OUTPUT_MAX_SIZE")
        .or_else(|| {
            root.as_ref()
                .and_then(|yaml| yaml_usize(yaml, "secret_backend_output_max_size"))
        })
        .unwrap_or(DEFAULT_MAX_OUTPUT_BYTES);
    let remove_trailing_line_break = env_bool("DD_SECRET_BACKEND_REMOVE_TRAILING_LINE_BREAK")
        .or_else(|| {
            root.as_ref()
                .and_then(|yaml| yaml_bool(yaml, "secret_backend_remove_trailing_line_break"))
        })
        .unwrap_or(false);
    Ok(Some(Backend {
        command,
        arguments,
        timeout_secs,
        max_output_bytes,
        remove_trailing_line_break,
    }))
}

fn load_agent_yaml_root(agent_yaml: &str) -> Result<Option<serde_yaml::Value>> {
    if !Path::new(agent_yaml).is_file() {
        return Ok(None);
    }
    let contents = std::fs::read_to_string(agent_yaml)
        .with_context(|| format!("read {agent_yaml} for secret backend config"))?;
    let root = yaml_load::load_yaml(&contents)
        .with_context(|| format!("parse {agent_yaml} for secret backend config"))?;
    Ok(Some(root))
}

fn env_string(name: &str) -> Option<String> {
    env_bindings::env_var_value_for_name(name)
}

fn env_u64(name: &str) -> Option<u64> {
    env_string(name).and_then(|text| text.parse().ok())
}

fn env_bool(name: &str) -> Option<bool> {
    env_string(name).and_then(|text| text.parse().ok())
}

fn env_usize(name: &str) -> Option<usize> {
    env_u64(name).and_then(|value| usize::try_from(value).ok())
}

fn env_string_list(name: &str) -> Option<Vec<String>> {
    env_string(name).map(|text| {
        if text.is_empty() {
            Vec::new()
        } else {
            text.split(' ').map(str::to_owned).collect()
        }
    })
}

fn fetch_secret(backend: &Backend, handle: &str) -> Result<String> {
    let payload = serde_json::json!({
        "version": PAYLOAD_VERSION,
        "secrets": [handle],
        "secret_backend_timeout": backend.timeout_secs,
    });
    let output = exec_backend(backend, &payload.to_string())?;
    parse_secret_response(&output, handle, backend.remove_trailing_line_break)
}

fn exec_backend(backend: &Backend, payload: &str) -> Result<String> {
    crate::platform::exec_secret_backend(
        &backend.command,
        &backend.arguments,
        payload,
        Duration::from_secs(backend.timeout_secs),
        backend.max_output_bytes,
    )
}

fn parse_secret_response(
    output: &str,
    handle: &str,
    remove_trailing_line_break: bool,
) -> Result<String> {
    let parsed: Value =
        serde_json::from_str(output).context("parse secret backend JSON response")?;
    let Some(entry) = parsed.get(handle) else {
        bail!("secret backend response missing handle {handle}");
    };
    if let Some(error) = entry.get("error").and_then(Value::as_str) {
        bail!("secret backend error for {handle}: {error}");
    }
    entry
        .get("value")
        .and_then(Value::as_str)
        .map(|value| normalize_secret_value(value, remove_trailing_line_break))
        .with_context(|| format!("secret backend response missing value for {handle}"))
}

fn normalize_secret_value(value: &str, remove_trailing_line_break: bool) -> String {
    if remove_trailing_line_break {
        value.trim_end_matches(['\r', '\n']).to_string()
    } else {
        value.to_owned()
    }
}

fn yaml_string(root: &serde_yaml::Value, key: &str) -> Option<String> {
    root.get(key)
        .and_then(|value| match value {
            serde_yaml::Value::String(text) => Some(text.clone()),
            serde_yaml::Value::Number(number) => Some(number.to_string()),
            serde_yaml::Value::Bool(enabled) => Some(enabled.to_string()),
            _ => None,
        })
        .filter(|text| !text.is_empty())
}

fn yaml_string_list(root: &serde_yaml::Value, key: &str) -> Vec<String> {
    root.get(key)
        .and_then(|value| value.as_sequence())
        .map(|items| {
            items
                .iter()
                .filter_map(|item| item.as_str().map(str::to_owned))
                .collect()
        })
        .unwrap_or_default()
}

fn yaml_u64(root: &serde_yaml::Value, key: &str) -> Option<u64> {
    root.get(key).and_then(|value| match value {
        serde_yaml::Value::Number(number) => number.as_u64(),
        serde_yaml::Value::String(text) => text.parse().ok(),
        _ => None,
    })
}

fn yaml_usize(root: &serde_yaml::Value, key: &str) -> Option<usize> {
    yaml_u64(root, key).and_then(|value| usize::try_from(value).ok())
}

fn yaml_bool(root: &serde_yaml::Value, key: &str) -> Option<bool> {
    root.get(key).and_then(|value| match value {
        serde_yaml::Value::Bool(enabled) => Some(*enabled),
        serde_yaml::Value::String(text) => text.parse().ok(),
        _ => None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Mutex, OnceLock};

    struct EnvGuard {
        name: String,
    }

    impl EnvGuard {
        fn set(name: &str, value: &str) -> Self {
            // SAFETY: tests acquire the env lock before calling set_var.
            unsafe { std::env::set_var(name, value) };
            Self {
                name: name.to_owned(),
            }
        }
    }

    impl Drop for EnvGuard {
        fn drop(&mut self) {
            // SAFETY: tests acquire the env lock before calling remove_var.
            unsafe { std::env::remove_var(&self.name) };
        }
    }

    fn with_env_lock<F: FnOnce()>(test: F) {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        let _guard = LOCK.get_or_init(|| Mutex::new(())).lock().unwrap();
        test();
    }

    #[test]
    fn enc_handle_parses_trimmed_values() {
        assert_eq!(
            enc_handle("ENC[process_enabled]"),
            Some("process_enabled".into())
        );
        assert_eq!(
            enc_handle("  ENC[ process_enabled ]  "),
            Some("process_enabled".into())
        );
        assert_eq!(enc_handle("true"), None);
        assert_eq!(enc_handle("ENC[]"), None);
    }

    #[test]
    fn resolve_config_string_leaves_literals_unchanged() {
        assert_eq!(
            resolve_config_string("true", "/nonexistent/datadog.yaml"),
            "true"
        );
    }

    #[test]
    fn resolve_config_string_uses_env_secret_backend_command() {
        with_env_lock(|| {
            let dir = tempfile::tempdir().unwrap();
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("env_secret_backend.sh");
                std::fs::write(
                    &path,
                    "#!/bin/sh\nprintf '{\"process_enabled\":{\"value\":\"true\"}}'\n",
                )
                .unwrap();
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
                path
            };
            #[cfg(windows)]
            let script = {
                let path = dir.path().join("env_secret_backend.cmd");
                std::fs::write(
                    &path,
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"process_enabled\\\":{\\\"value\\\":\\\"true\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(&agent_yaml, "# no secret backend in yaml\n").unwrap();
            let _backend = EnvGuard::set(
                "DD_SECRET_BACKEND_COMMAND",
                script.to_string_lossy().as_ref(),
            );

            assert_eq!(
                resolve_config_string("ENC[process_enabled]", agent_yaml.to_str().unwrap()),
                "true"
            );
        });
    }

    #[test]
    fn normalize_secret_value_strips_trailing_line_breaks_when_enabled() {
        assert_eq!(normalize_secret_value("true\r\n", true), "true".to_string());
        assert_eq!(normalize_secret_value("true\n", true), "true".to_string());
        assert_eq!(
            normalize_secret_value("true\r\n", false),
            "true\r\n".to_string()
        );
    }

    #[test]
    fn clear_caches_drops_cached_handles() {
        cache_handle("process_enabled", "true");
        assert_eq!(cached_handle("process_enabled"), Some("true".into()));
        clear_caches();
        assert_eq!(cached_handle("process_enabled"), None);
    }

    #[test]
    fn exec_backend_times_out_hung_command() {
        let dir = tempfile::tempdir().unwrap();
        #[cfg(unix)]
        let script = {
            let path = dir.path().join("slow_secret_backend.sh");
            std::fs::write(&path, "#!/bin/sh\nsleep 30\n").unwrap();
            use std::os::unix::fs::PermissionsExt;
            std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
            path
        };
        #[cfg(windows)]
        let script = {
            let path = dir.path().join("slow_secret_backend.cmd");
            std::fs::write(&path, "@echo off\r\nping -n 30 127.0.0.1 >nul\r\n").unwrap();
            path
        };
        let agent_yaml = dir.path().join("datadog.yaml");
        std::fs::write(
            &agent_yaml,
            format!(
                "secret_backend_command: {}\nsecret_backend_timeout: 1\n",
                script.to_string_lossy()
            ),
        )
        .unwrap();

        let started = std::time::Instant::now();
        assert_eq!(
            resolve_config_string("ENC[slow]", agent_yaml.to_str().unwrap()),
            "ENC[slow]"
        );
        assert!(
            started.elapsed() < Duration::from_secs(5),
            "hung secret backend should time out quickly"
        );
    }
}
