// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolve `ENC[...]` config values via `secret_backend_command`.
//!
//! Mirrors `comp/core/secrets/utils.IsEnc` and the command-backend payload in
//! `comp/core/secrets/impl/fetch_secret.go`. Only `secret_backend_command` is
//! supported (not `secret_backend_type` / `multi_secret_backends`).

use std::collections::HashMap;
use std::io::Write;
use std::path::Path;
use std::process::{Command, Stdio};
use std::sync::{Mutex, OnceLock};

use anyhow::{Context, Result, bail};
use log::debug;
use serde_json::Value;

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
        bail!("secret_backend_command is not configured in {agent_yaml}");
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
    if !Path::new(agent_yaml).is_file() {
        return Ok(None);
    }
    let contents = std::fs::read_to_string(agent_yaml)
        .with_context(|| format!("read {agent_yaml} for secret backend config"))?;
    let root = yaml_load::load_yaml(&contents)
        .with_context(|| format!("parse {agent_yaml} for secret backend config"))?;
    let Some(command) = yaml_string(&root, "secret_backend_command") else {
        return Ok(None);
    };
    if command.trim().is_empty() {
        return Ok(None);
    }
    Ok(Some(Backend {
        command,
        arguments: yaml_string_list(&root, "secret_backend_arguments"),
        timeout_secs: yaml_u64(&root, "secret_backend_timeout").unwrap_or(DEFAULT_TIMEOUT_SECS),
        max_output_bytes: yaml_usize(&root, "secret_backend_output_max_size")
            .unwrap_or(DEFAULT_MAX_OUTPUT_BYTES),
    }))
}

fn fetch_secret(backend: &Backend, handle: &str) -> Result<String> {
    let payload = serde_json::json!({
        "version": PAYLOAD_VERSION,
        "secrets": [handle],
        "secret_backend_timeout": backend.timeout_secs,
    });
    let output = exec_backend(backend, &payload.to_string())?;
    parse_secret_response(&output, handle)
}

fn exec_backend(backend: &Backend, payload: &str) -> Result<String> {
    let mut command = Command::new(&backend.command);
    command
        .args(&backend.arguments)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    let mut child = command
        .spawn()
        .with_context(|| format!("spawn secret backend {}", backend.command))?;
    if let Some(mut stdin) = child.stdin.take() {
        stdin
            .write_all(payload.as_bytes())
            .context("write secret backend payload")?;
    }
    let output = child
        .wait_with_output()
        .context("wait for secret backend")?;
    if !output.status.success() {
        bail!(
            "secret backend {} exited with {}",
            backend.command,
            output.status
        );
    }
    if output.stdout.len() > backend.max_output_bytes {
        bail!(
            "secret backend output exceeded {} bytes",
            backend.max_output_bytes
        );
    }
    String::from_utf8(output.stdout).context("decode secret backend stdout as UTF-8")
}

fn parse_secret_response(output: &str, handle: &str) -> Result<String> {
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
        .map(str::to_owned)
        .with_context(|| format!("secret backend response missing value for {handle}"))
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

#[cfg(test)]
mod tests {
    use super::*;

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
}
