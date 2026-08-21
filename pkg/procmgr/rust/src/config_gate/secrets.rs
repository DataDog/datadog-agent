// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve `ENC[...]` config values via the Agent secret backend.
//!
//! Mirrors `comp/core/secrets/utils.IsEnc` and `fetch_secret.go`. Supports
//! `secret_backend_command`, `secret_backend_type`, and `multi_secret_backends`
//! with the same precedence as the core Agent (command > type > multi), invoking
//! `secret-generic-connector` for native backends.
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
struct MultiBackendEntry {
    backend_type: String,
    config: Option<Value>,
}

#[derive(Clone)]
struct Backend {
    command: String,
    arguments: Vec<String>,
    skip_acl_check: bool,
    multi_backends: Option<HashMap<String, MultiBackendEntry>>,
    global_backend_type: Option<String>,
    global_backend_config: Option<Value>,
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
    let inner = trimmed.strip_prefix("ENC[")?.strip_suffix(']')?;
    Some(inner.to_owned())
}

fn resolve_handle(handle: &str, agent_yaml: &str) -> Result<String> {
    if let Some(cached) = cached_handle(handle) {
        return Ok(cached);
    }
    let Some(backend) = backend_for(agent_yaml)? else {
        bail!("no secret backend is configured");
    };
    let (backend_type, backend_config, secret_key) = route_handle(&backend, handle)?;
    let value = fetch_secret(
        &backend,
        &secret_key,
        backend_type.as_deref(),
        backend_config.as_ref(),
    )?;
    cache_handle(handle, &value);
    Ok(value)
}

fn route_handle(
    backend: &Backend,
    handle: &str,
) -> Result<(Option<String>, Option<Value>, String)> {
    if backend.multi_backends.is_some() {
        let (backend_id, secret_key) = split_secret_handle(handle);
        if backend_id.is_empty() {
            if backend.global_backend_type.is_none() {
                bail!("unknown backend");
            }
            return Ok((
                backend.global_backend_type.clone(),
                backend.global_backend_config.clone(),
                secret_key.to_owned(),
            ));
        }
        let entry = backend
            .multi_backends
            .as_ref()
            .and_then(|backends| backends.get(&backend_id.to_ascii_lowercase()))
            .with_context(|| format!("unknown backend {backend_id:?}"))?;
        return Ok((
            Some(entry.backend_type.clone()),
            entry.config.clone(),
            secret_key.to_owned(),
        ));
    }
    Ok((
        backend.global_backend_type.clone(),
        backend.global_backend_config.clone(),
        handle.to_owned(),
    ))
}

fn split_secret_handle(handle: &str) -> (&str, &str) {
    match handle.split_once(';') {
        Some((backend_id, secret_key)) => (backend_id, secret_key),
        None => ("", handle),
    }
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
    let settings = common_backend_settings(root.as_ref())?;

    if let Some(command) = load_command(root.as_ref()) {
        return Ok(Some(Backend {
            command,
            arguments: load_arguments(root.as_ref()),
            skip_acl_check: false,
            multi_backends: None,
            global_backend_type: None,
            global_backend_config: None,
            ..settings
        }));
    }

    if let Some(backend_type) = load_backend_type(root.as_ref()) {
        return Ok(Some(embedded_backend(
            settings,
            Some(backend_type),
            load_backend_config(root.as_ref()),
            None,
        )));
    }

    if let Some(multi) = load_multi_backends(root.as_ref()) {
        return Ok(Some(embedded_backend(settings, None, None, Some(multi))));
    }

    Ok(None)
}

fn embedded_backend(
    settings: Backend,
    global_backend_type: Option<String>,
    global_backend_config: Option<Value>,
    multi_backends: Option<HashMap<String, MultiBackendEntry>>,
) -> Backend {
    Backend {
        command: crate::platform::embedded_secret_connector_path()
            .to_string_lossy()
            .into_owned(),
        arguments: settings.arguments,
        skip_acl_check: true,
        multi_backends,
        global_backend_type,
        global_backend_config,
        timeout_secs: settings.timeout_secs,
        max_output_bytes: settings.max_output_bytes,
        remove_trailing_line_break: settings.remove_trailing_line_break,
    }
}

fn common_backend_settings(root: Option<&serde_yaml::Value>) -> Result<Backend> {
    Ok(Backend {
        command: String::new(),
        arguments: load_arguments(root),
        skip_acl_check: false,
        multi_backends: None,
        global_backend_type: None,
        global_backend_config: None,
        timeout_secs: env_u64("DD_SECRET_BACKEND_TIMEOUT")
            .or_else(|| root.and_then(|yaml| yaml_u64(yaml, "secret_backend_timeout")))
            .unwrap_or(DEFAULT_TIMEOUT_SECS),
        max_output_bytes: env_usize("DD_SECRET_BACKEND_OUTPUT_MAX_SIZE")
            .or_else(|| root.and_then(|yaml| yaml_usize(yaml, "secret_backend_output_max_size")))
            .unwrap_or(DEFAULT_MAX_OUTPUT_BYTES),
        remove_trailing_line_break: env_bool("DD_SECRET_BACKEND_REMOVE_TRAILING_LINE_BREAK")
            .or_else(|| {
                root.and_then(|yaml| yaml_bool(yaml, "secret_backend_remove_trailing_line_break"))
            })
            .unwrap_or(false),
    })
}

fn load_command(root: Option<&serde_yaml::Value>) -> Option<String> {
    env_string("DD_SECRET_BACKEND_COMMAND")
        .or_else(|| root.and_then(|yaml| yaml_string(yaml, "secret_backend_command")))
        .filter(|command| !command.trim().is_empty())
}

fn load_backend_type(root: Option<&serde_yaml::Value>) -> Option<String> {
    env_string("DD_SECRET_BACKEND_TYPE")
        .or_else(|| root.and_then(|yaml| yaml_string(yaml, "secret_backend_type")))
        .filter(|backend_type| !backend_type.trim().is_empty())
}

fn load_backend_config(root: Option<&serde_yaml::Value>) -> Option<Value> {
    match env_string("DD_SECRET_BACKEND_CONFIG") {
        Some(text) => parse_env_json_map(&text),
        None => root
            .and_then(|yaml| yaml_get(yaml, "secret_backend_config"))
            .and_then(|value| serde_json::to_value(value).ok()),
    }
    .filter(|value| !value.is_null())
}

fn parse_env_json_map(text: &str) -> Option<Value> {
    let trimmed = text.trim();
    if trimmed.is_empty() {
        return None;
    }
    match serde_json::from_str::<Value>(trimmed) {
        Ok(value) if !value.is_null() => Some(value),
        Ok(_) => None,
        Err(err) => {
            debug!("ignore invalid JSON in DD_SECRET_BACKEND_CONFIG: {err}");
            None
        }
    }
}

fn load_multi_backends(
    root: Option<&serde_yaml::Value>,
) -> Option<HashMap<String, MultiBackendEntry>> {
    let mapping = yaml_get(root?, "multi_secret_backends")?.as_mapping()?;
    let mut backends = HashMap::new();
    for (name, entry) in mapping {
        let Some(name) = name.as_str() else {
            continue;
        };
        let Some(entry) = entry.as_mapping() else {
            continue;
        };
        let backend_type = super::lookup_mapping_case_insensitive(entry, "type")
            .and_then(|value| value.as_str())
            .filter(|text| !text.is_empty())?;
        let config = super::lookup_mapping_case_insensitive(entry, "config")
            .and_then(|value| serde_json::to_value(value).ok());
        backends.insert(
            name.to_ascii_lowercase(),
            MultiBackendEntry {
                backend_type: backend_type.to_owned(),
                config,
            },
        );
    }
    (!backends.is_empty()).then_some(backends)
}

fn load_arguments(root: Option<&serde_yaml::Value>) -> Vec<String> {
    env_string_list("DD_SECRET_BACKEND_ARGUMENTS")
        .or_else(|| root.map(|yaml| yaml_string_list(yaml, "secret_backend_arguments")))
        .unwrap_or_default()
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
    env_string(name)
        .as_deref()
        .and_then(super::parse_agent_bool_string)
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

fn fetch_secret(
    backend: &Backend,
    secret_key: &str,
    backend_type: Option<&str>,
    backend_config: Option<&Value>,
) -> Result<String> {
    let mut payload = serde_json::json!({
        "version": PAYLOAD_VERSION,
        "secrets": [secret_key],
        "secret_backend_timeout": backend.timeout_secs,
    });
    if let Some(backend_type) = backend_type.filter(|text| !text.is_empty()) {
        payload["type"] = Value::String(backend_type.to_owned());
    }
    if let Some(config) = backend_config.filter(|value| !value.is_null()) {
        payload["config"] = config.clone();
    }
    let output = exec_backend(backend, &payload.to_string())?;
    parse_secret_response(&output, secret_key, backend.remove_trailing_line_break)
}

fn exec_backend(backend: &Backend, payload: &str) -> Result<String> {
    crate::platform::exec_secret_backend(
        &backend.command,
        &backend.arguments,
        payload,
        Duration::from_secs(backend.timeout_secs),
        backend.max_output_bytes,
        backend.skip_acl_check,
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
    if let Some(error) = entry.get("error").and_then(Value::as_str)
        && !error.is_empty()
    {
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

fn yaml_get<'a>(root: &'a serde_yaml::Value, key: &str) -> Option<&'a serde_yaml::Value> {
    let mapping = root.as_mapping()?;
    super::lookup_mapping_case_insensitive(mapping, key)
}

fn yaml_string(root: &serde_yaml::Value, key: &str) -> Option<String> {
    yaml_get(root, key)
        .and_then(|value| match value {
            serde_yaml::Value::String(text) => Some(text.clone()),
            serde_yaml::Value::Number(number) => Some(number.to_string()),
            serde_yaml::Value::Bool(enabled) => Some(enabled.to_string()),
            _ => None,
        })
        .filter(|text| !text.is_empty())
}

fn yaml_string_list(root: &serde_yaml::Value, key: &str) -> Vec<String> {
    yaml_get(root, key)
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
    yaml_get(root, key).and_then(|value| match value {
        serde_yaml::Value::Number(number) => number.as_u64(),
        serde_yaml::Value::String(text) => text.parse().ok(),
        _ => None,
    })
}

fn yaml_usize(root: &serde_yaml::Value, key: &str) -> Option<usize> {
    yaml_u64(root, key).and_then(|value| usize::try_from(value).ok())
}

fn yaml_bool(root: &serde_yaml::Value, key: &str) -> Option<bool> {
    yaml_get(root, key).and_then(|value| match value {
        serde_yaml::Value::Bool(enabled) => Some(*enabled),
        serde_yaml::Value::Number(number) => number.as_f64().map(|n| n != 0.0),
        serde_yaml::Value::String(text) => super::parse_agent_bool_string(text),
        _ => None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config_gate::test_env;

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
        test_env::with_lock(|| {
            test_env::clear_secret_backend_env_vars();
            test();
        });
    }

    #[test]
    fn enc_handle_parses_trimmed_values() {
        assert_eq!(
            enc_handle("ENC[process_enabled]"),
            Some("process_enabled".into())
        );
        assert_eq!(
            enc_handle("  ENC[ process_enabled ]  "),
            Some(" process_enabled ".into())
        );
        assert_eq!(enc_handle("true"), None);
        assert_eq!(enc_handle("ENC[]"), Some(String::new()));
        assert_eq!(enc_handle("ENC[]]]]"), Some("]]]".into()));
    }

    #[test]
    fn split_secret_handle_splits_backend_id_and_key() {
        assert_eq!(
            split_secret_handle("file;process_enabled"),
            ("file", "process_enabled")
        );
        assert_eq!(
            split_secret_handle("process_enabled"),
            ("", "process_enabled")
        );
    }

    #[test]
    fn parse_secret_response_ignores_empty_error_field() {
        let output = r#"{"process_enabled":{"value":"true","error":""}}"#;
        assert_eq!(
            parse_secret_response(output, "process_enabled", false).unwrap(),
            "true"
        );
    }

    #[test]
    fn parse_secret_response_rejects_non_empty_error_field() {
        let output = r#"{"process_enabled":{"value":"true","error":"backend failed"}}"#;
        let err = parse_secret_response(output, "process_enabled", false).unwrap_err();
        assert!(
            err.to_string().contains("backend failed"),
            "unexpected error: {err:#}"
        );
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
            clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
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
    fn load_backend_uses_mixed_case_secret_backend_keys() {
        with_env_lock(|| {
            clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("mixed_case_secret_backend.sh");
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
                let path = dir.path().join("mixed_case_secret_backend.cmd");
                std::fs::write(
                    &path,
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"process_enabled\\\":{\\\"value\\\":\\\"true\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                format!("Secret_Backend_Command: {}\n", script.to_string_lossy()),
            )
            .unwrap();

            assert_eq!(
                resolve_config_string("ENC[process_enabled]", agent_yaml.to_str().unwrap()),
                "true"
            );
        });
    }

    #[test]
    fn load_backend_parses_mixed_case_multi_secret_backends() {
        with_env_lock(|| {
            clear_caches();
            let dir = tempfile::tempdir().unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                "Multi_Secret_Backends:\n  file:\n    Type: file.yaml\n    Config:\n      file_path: /tmp/secrets.yaml\n",
            )
            .unwrap();

            let backend = load_backend(agent_yaml.to_str().unwrap())
                .unwrap()
                .expect("backend");
            let entry = backend
                .multi_backends
                .as_ref()
                .and_then(|backends| backends.get("file"))
                .expect("file backend");
            assert_eq!(entry.backend_type, "file.yaml");
            assert_eq!(
                entry.config.as_ref().and_then(|c| c.get("file_path")),
                Some(&Value::String("/tmp/secrets.yaml".into()))
            );
        });
    }

    #[test]
    fn load_backend_prefers_command_over_secret_backend_type() {
        with_env_lock(|| {
            clear_caches();
            let dir = tempfile::tempdir().unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                "secret_backend_type: file.json\nsecret_backend_command: /custom/backend\n",
            )
            .unwrap();
            let _command = EnvGuard::set("DD_SECRET_BACKEND_COMMAND", "/env/backend");

            let backend = load_backend(agent_yaml.to_str().unwrap())
                .unwrap()
                .expect("backend");
            assert_eq!(backend.command, "/env/backend");
            assert!(!backend.skip_acl_check);
            assert!(backend.global_backend_type.is_none());
        });
    }

    #[test]
    fn load_backend_uses_embedded_connector_for_secret_backend_type() {
        with_env_lock(|| {
            clear_caches();
            let dir = tempfile::tempdir().unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                "secret_backend_type: file.json\nsecret_backend_config:\n  file_path: /tmp/secrets.json\n",
            )
            .unwrap();

            let backend = load_backend(agent_yaml.to_str().unwrap())
                .unwrap()
                .expect("backend");
            assert_eq!(
                backend.command,
                crate::platform::embedded_secret_connector_path()
                    .to_string_lossy()
                    .as_ref()
            );
            assert!(backend.skip_acl_check);
            assert_eq!(backend.global_backend_type.as_deref(), Some("file.json"));
            assert_eq!(
                backend
                    .global_backend_config
                    .as_ref()
                    .and_then(|c| c.get("file_path")),
                Some(&Value::String("/tmp/secrets.json".into()))
            );
        });
    }

    #[test]
    fn load_backend_uses_env_secret_backend_config() {
        let connector = crate::platform::embedded_secret_connector_path();
        if !connector.is_file() {
            return;
        }

        with_env_lock(|| {
            clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
            let secrets_file = dir.path().join("secrets.json");
            std::fs::write(&secrets_file, r#"{"process_enabled": "true"}"#).unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                "secret_backend_type: file.json\nsecret_backend_config:\n  file_path: /nonexistent/secrets.json\n",
            )
            .unwrap();
            let _backend_type = EnvGuard::set("DD_SECRET_BACKEND_TYPE", "file.json");
            let _backend_config = EnvGuard::set(
                "DD_SECRET_BACKEND_CONFIG",
                &format!(r#"{{"file_path":"{}"}}"#, secrets_file.to_string_lossy()),
            );

            let backend = load_backend(agent_yaml.to_str().unwrap())
                .unwrap()
                .expect("backend");
            assert_eq!(backend.global_backend_type.as_deref(), Some("file.json"));
            assert_eq!(
                backend
                    .global_backend_config
                    .as_ref()
                    .and_then(|c| c.get("file_path")),
                Some(&Value::String(secrets_file.to_string_lossy().into_owned()))
            );
            assert_eq!(
                resolve_config_string("ENC[process_enabled]", agent_yaml.to_str().unwrap()),
                "true"
            );
        });
    }

    #[test]
    fn load_backend_parses_multi_secret_backends() {
        with_env_lock(|| {
            clear_caches();
            let dir = tempfile::tempdir().unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                "multi_secret_backends:\n  file:\n    type: file.yaml\n    config:\n      file_path: /tmp/secrets.yaml\n",
            )
            .unwrap();

            let backend = load_backend(agent_yaml.to_str().unwrap())
                .unwrap()
                .expect("backend");
            assert_eq!(
                backend.command,
                crate::platform::embedded_secret_connector_path()
                    .to_string_lossy()
                    .as_ref()
            );
            assert!(backend.skip_acl_check);
            let entry = backend
                .multi_backends
                .as_ref()
                .and_then(|backends| backends.get("file"))
                .expect("file backend");
            assert_eq!(entry.backend_type, "file.yaml");
            assert_eq!(
                entry.config.as_ref().and_then(|c| c.get("file_path")),
                Some(&Value::String("/tmp/secrets.yaml".into()))
            );
        });
    }

    #[test]
    fn resolve_config_string_uses_native_file_json_backend() {
        let connector = crate::platform::embedded_secret_connector_path();
        if !connector.is_file() {
            return;
        }

        with_env_lock(|| {
            clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
            let secrets_file = dir.path().join("secrets.json");
            std::fs::write(&secrets_file, r#"{"process_enabled": "true"}"#).unwrap();
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(
                &agent_yaml,
                format!(
                    "secret_backend_type: file.json\nsecret_backend_config:\n  file_path: {}\n",
                    secrets_file.to_string_lossy()
                ),
            )
            .unwrap();

            assert_eq!(
                resolve_config_string("ENC[process_enabled]", agent_yaml.to_str().unwrap()),
                "true"
            );
        });
    }

    #[test]
    fn route_handle_resolves_named_multi_backend_entry() {
        let backend = Backend {
            command: "ignored".into(),
            arguments: Vec::new(),
            skip_acl_check: true,
            multi_backends: Some(HashMap::from([(
                "file".to_string(),
                MultiBackendEntry {
                    backend_type: "file.yaml".into(),
                    config: Some(serde_json::json!({"file_path": "/tmp/secrets.yaml"})),
                },
            )])),
            global_backend_type: None,
            global_backend_config: None,
            timeout_secs: DEFAULT_TIMEOUT_SECS,
            max_output_bytes: DEFAULT_MAX_OUTPUT_BYTES,
            remove_trailing_line_break: false,
        };

        let (backend_type, backend_config, secret_key) =
            route_handle(&backend, "file;process_enabled").unwrap();
        assert_eq!(backend_type.as_deref(), Some("file.yaml"));
        assert_eq!(secret_key, "process_enabled");
        assert_eq!(
            backend_config.as_ref().and_then(|c| c.get("file_path")),
            Some(&Value::String("/tmp/secrets.yaml".into()))
        );
    }

    #[test]
    fn route_handle_rejects_unprefixed_handle_for_multi_only_backends() {
        let backend = Backend {
            command: "ignored".into(),
            arguments: Vec::new(),
            skip_acl_check: true,
            multi_backends: Some(HashMap::from([(
                "file".to_string(),
                MultiBackendEntry {
                    backend_type: "file.yaml".into(),
                    config: None,
                },
            )])),
            global_backend_type: None,
            global_backend_config: None,
            timeout_secs: DEFAULT_TIMEOUT_SECS,
            max_output_bytes: DEFAULT_MAX_OUTPUT_BYTES,
            remove_trailing_line_break: false,
        };

        assert!(route_handle(&backend, "process_enabled").is_err());
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
    fn remove_trailing_line_break_honors_agent_bool_env_spelling() {
        with_env_lock(|| {
            clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("newline_secret_backend.sh");
                std::fs::write(
                    &path,
                    "#!/bin/sh\ncat <<'EOF'\n{\"process_enabled\":{\"value\":\"true\\n\"}}\nEOF\n",
                )
                .unwrap();
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
                path
            };
            #[cfg(windows)]
            let script = {
                let path = dir.path().join("newline_secret_backend.cmd");
                std::fs::write(
                    &path,
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"process_enabled\\\":{\\\"value\\\":\\\"true`n\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent_yaml = dir.path().join("datadog.yaml");
            std::fs::write(&agent_yaml, "# no secret backend in yaml\n").unwrap();
            let _command = EnvGuard::set(
                "DD_SECRET_BACKEND_COMMAND",
                script.to_string_lossy().as_ref(),
            );
            let _strip = EnvGuard::set("DD_SECRET_BACKEND_REMOVE_TRAILING_LINE_BREAK", "1");

            assert_eq!(
                resolve_config_string("ENC[process_enabled]", agent_yaml.to_str().unwrap()),
                "true"
            );
        });
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
        let dir = test_env::tempdir_for_secret_backend();
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
