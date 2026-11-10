// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::warn;
use std::io::ErrorKind;
use std::path::Path;
use std::sync::OnceLock;

use crate::agent_yaml;

/// Supervisor-private IPC cert path. The `DD_PM_` prefix keeps it out of same-user
/// children, so it cannot override `ipc_cert_file_path` the way `DD_IPC_CERT_FILE_PATH` would.
pub const IPC_CERT_FILE_PATH_VAR: &str = "DD_PM_IPC_CERT_FILE_PATH";

const AGENT_CONFIG_PATH: &str = "/etc/datadog-agent/datadog.yaml";
const IPC_CERT_FILE_NAME: &str = "ipc_cert.pem";

static IPC_CERT_FILE_PATH: OnceLock<String> = OnceLock::new();

fn lookup_env(name: &str) -> Option<String> {
    if name != IPC_CERT_FILE_PATH_VAR {
        return std::env::var(name).ok();
    }
    ipc_cert_file_path()
        .inspect_err(|e| warn!("could not resolve the Agent IPC certificate path: {e:#}"))
        .ok()
}

fn ipc_cert_file_path() -> Result<String> {
    if let Some(value) = std::env::var(IPC_CERT_FILE_PATH_VAR)
        .ok()
        .filter(|value| !value.is_empty())
    {
        return Ok(value);
    }
    cached_ipc_cert_file_path(&IPC_CERT_FILE_PATH, || {
        resolve_ipc_cert_file_path(Path::new(AGENT_CONFIG_PATH), |name| {
            std::env::var(name).ok()
        })
    })
}

/// Only a success is cached, so a failure is retried on the next spawn.
fn cached_ipc_cert_file_path(
    cache: &OnceLock<String>,
    resolve: impl FnOnce() -> Result<String>,
) -> Result<String> {
    if let Some(cached) = cache.get() {
        return Ok(cached.clone());
    }
    let path = resolve()?;
    Ok(cache.get_or_init(|| path).clone())
}

/// Expands like [`expand_env_vars`], but fails when the input references the IPC
/// certificate path and it cannot be resolved. Without this the placeholder stays
/// literal, and `run-executor` would treat it as a path and create a certificate there.
pub(crate) fn expand_env_vars_checked(input: &str) -> Result<String> {
    require_ipc_cert_file_path(input, ipc_cert_file_path)?;
    Ok(expand_env_vars(input))
}

fn require_ipc_cert_file_path(input: &str, resolve: impl FnOnce() -> Result<String>) -> Result<()> {
    if input.contains(&format!("${{{IPC_CERT_FILE_PATH_VAR}}}")) {
        resolve().context("resolving the Agent IPC certificate path")?;
    }
    Ok(())
}

/// Resolves the IPC certificate path in the order the Agent uses.
///
/// 1. `DD_IPC_CERT_FILE_PATH`
/// 2. `ipc_cert_file_path` in `config_path`
/// 3. the directory of `DD_AUTH_TOKEN_FILE_PATH`, else of `auth_token_file_path`
/// 4. the directory of `config_path`
///
/// The last two join `ipc_cert.pem`, like `getCertFilepath` in `pkg/api/security/cert`.
/// A missing config reads as empty. A parse error or an `ENC[...]` handle is an error,
/// so the path is never guessed.
pub(crate) fn resolve_ipc_cert_file_path(
    config_path: &Path,
    env: impl Fn(&str) -> Option<String>,
) -> Result<String> {
    let non_empty_env =
        |name: &str| reject_secret_handle(name, env(name).filter(|value| !value.is_empty()));
    if let Some(path) = non_empty_env("DD_IPC_CERT_FILE_PATH")? {
        return Ok(path);
    }

    let root = match std::fs::read_to_string(config_path) {
        Ok(contents) => agent_yaml::load(&contents)
            .with_context(|| format!("parsing {}", config_path.display()))?,
        Err(e) if e.kind() == ErrorKind::NotFound => serde_yaml::Value::Null,
        Err(e) => {
            return Err(e).with_context(|| format!("reading {}", config_path.display()));
        }
    };
    let yaml_string = |key: &str| {
        let value = agent_yaml::lookup_dotted_key(&root, key)
            .and_then(agent_yaml::scalar_as_string)
            .filter(|value| !value.is_empty());
        reject_secret_handle(key, value)
    };

    if let Some(path) = yaml_string("ipc_cert_file_path")? {
        return Ok(path);
    }
    let token_path = match non_empty_env("DD_AUTH_TOKEN_FILE_PATH")? {
        Some(path) => Some(path),
        None => yaml_string("auth_token_file_path")?,
    };
    let dir = match token_path.as_deref().map(Path::new) {
        Some(token) => token.parent(),
        None => config_path.parent(),
    }
    .unwrap_or_else(|| Path::new("/"));
    Ok(dir.join(IPC_CERT_FILE_NAME).to_string_lossy().into_owned())
}

/// Matches `IsEnc` in `comp/core/secrets/utils`.
fn reject_secret_handle(setting: &str, value: Option<String>) -> Result<Option<String>> {
    if let Some(value) = &value {
        let trimmed = value.trim_matches([' ', '\t']);
        if trimmed.starts_with("ENC[") && trimmed.ends_with(']') {
            anyhow::bail!(
                "{setting} is a secret handle that dd-procmgrd cannot resolve, set DD_IPC_CERT_FILE_PATH to the certificate path"
            );
        }
    }
    Ok(value)
}

pub(crate) fn expand_env_vars(input: &str) -> String {
    expand_vars_with(input, lookup_env)
}

pub(crate) fn try_expand_env_vars(input: &str) -> Option<String> {
    try_expand_vars_with(input, lookup_env)
}

pub(crate) fn expand_vars_with(input: &str, lookup: impl Fn(&str) -> Option<String>) -> String {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(start) = rest.find("${") {
        out.push_str(&rest[..start]);
        let after = &rest[start + 2..];
        match after.find('}') {
            Some(end) => {
                let name = &after[..end];
                match lookup(name) {
                    Some(val) => out.push_str(&val),
                    None => {
                        warn!(
                            "process config references unset variable ${{{name}}}, leaving it literal"
                        );
                        out.push_str(&rest[start..start + 2 + end + 1]);
                    }
                }
                rest = &after[end + 1..];
            }
            None => {
                out.push_str(&rest[start..]);
                return out;
            }
        }
    }
    out.push_str(rest);
    out
}

pub(crate) fn try_expand_vars_with(
    input: &str,
    lookup: impl Fn(&str) -> Option<String>,
) -> Option<String> {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(start) = rest.find("${") {
        out.push_str(&rest[..start]);
        let after = &rest[start + 2..];
        let end = after.find('}')?;
        let name = &after[..end];
        out.push_str(&lookup(name)?);
        rest = &after[end + 1..];
    }
    out.push_str(rest);
    Some(out)
}

pub fn parse_environment_file(path: &str) -> Result<Vec<(String, String)>> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("reading environment file: {path}"))?;
    let mut vars = Vec::new();
    for line in contents.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        if let Some((key, raw_val)) = trimmed.split_once('=') {
            let val = raw_val
                .trim()
                .trim_matches('"')
                .trim_matches('\'')
                .to_string();
            vars.push((key.trim().to_string(), val));
        }
    }
    Ok(vars)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::path::PathBuf;

    #[test]
    fn test_parse_full() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("env");
        std::fs::write(
            &path,
            r#"# Datadog env
DD_API_KEY=abc123
PATH="/usr/local/bin:/usr/bin"
QUOTED='single'
malformed line without equals
   
# blank lines above are skipped
LANG=en_US.UTF-8
"#,
        )
        .unwrap();

        let vars: HashMap<String, String> = parse_environment_file(path.to_str().unwrap())
            .unwrap()
            .into_iter()
            .collect();

        assert_eq!(vars["DD_API_KEY"], "abc123");
        assert_eq!(vars["PATH"], "/usr/local/bin:/usr/bin");
        assert_eq!(vars["QUOTED"], "single");
        assert_eq!(vars["LANG"], "en_US.UTF-8");
        assert_eq!(vars.len(), 4, "malformed line should be silently skipped");
    }

    #[test]
    fn test_parse_missing_file() {
        assert!(parse_environment_file("/nonexistent/env").is_err());
    }

    #[test]
    fn test_expand_vars_substitutes_known() {
        let lookup = |name: &str| match name {
            "DD_CONF_DIR" => Some("/etc/datadog-agent-exp".to_string()),
            _ => None,
        };
        assert_eq!(
            expand_vars_with("${DD_CONF_DIR}/otel-config.yaml", lookup),
            "/etc/datadog-agent-exp/otel-config.yaml"
        );
        assert_eq!(
            expand_vars_with("-${DD_CONF_DIR}/environment", lookup),
            "-/etc/datadog-agent-exp/environment"
        );
    }

    #[test]
    fn test_expand_vars_leaves_unknown_literal() {
        let lookup = |_: &str| None;
        assert_eq!(
            expand_vars_with("${MISSING}/x", lookup),
            "${MISSING}/x",
            "unset variables must be left literal so misconfiguration fails loudly"
        );
    }

    #[test]
    fn test_try_expand_vars_substitutes_known() {
        let lookup = |name: &str| match name {
            "DD_INVENTORIES_FIRST_RUN_DELAY" => Some("5".to_string()),
            _ => None,
        };
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            Some("5".to_string())
        );
    }

    #[test]
    fn test_try_expand_vars_none_when_unset() {
        let lookup = |_: &str| None;
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            None,
            "an unset optional variable must yield None so the caller can omit the env var"
        );
    }

    /// Returns the result and the temp dir, so expectations use `Path::join`.
    fn resolve(yaml: Option<&str>, env: &[(&str, &str)]) -> (Result<String>, PathBuf) {
        let dir = tempfile::tempdir().unwrap();
        let config_dir = dir.path().join("etc");
        let config = config_dir.join("datadog.yaml");
        if let Some(yaml) = yaml {
            std::fs::create_dir_all(&config_dir).unwrap();
            std::fs::write(&config, yaml).unwrap();
        }
        let env: HashMap<String, String> = env
            .iter()
            .map(|(name, value)| (name.to_string(), value.to_string()))
            .collect();
        let resolved = resolve_ipc_cert_file_path(&config, |name| env.get(name).cloned());
        (resolved, config_dir)
    }

    fn cert_in(dir: impl AsRef<Path>) -> String {
        dir.as_ref()
            .join(IPC_CERT_FILE_NAME)
            .to_string_lossy()
            .into_owned()
    }

    #[test]
    fn ipc_cert_env_outranks_the_yaml() {
        let (path, _) = resolve(
            Some("ipc_cert_file_path: /yaml/cert.pem\n"),
            &[("DD_IPC_CERT_FILE_PATH", "/env/cert.pem")],
        );
        assert_eq!(path.unwrap(), "/env/cert.pem");
    }

    #[test]
    fn ipc_cert_yaml_setting_is_kept() {
        let (path, _) = resolve(
            Some("ipc_cert_file_path: /yaml/cert.pem\nauth_token_file_path: /token/auth\n"),
            &[("DD_AUTH_TOKEN_FILE_PATH", "/env/token/auth")],
        );
        assert_eq!(path.unwrap(), "/yaml/cert.pem");
    }

    #[test]
    fn ipc_cert_follows_the_auth_token_directory() {
        let (path, _) = resolve(
            Some("auth_token_file_path: /yaml/token/auth\n"),
            &[("DD_AUTH_TOKEN_FILE_PATH", "/env/token/auth")],
        );
        assert_eq!(
            path.unwrap(),
            cert_in("/env/token"),
            "the auth token env outranks the YAML key"
        );
        let (path, _) = resolve(Some("auth_token_file_path: /yaml/token/auth\n"), &[]);
        assert_eq!(path.unwrap(), cert_in("/yaml/token"));
    }

    #[test]
    fn ipc_cert_defaults_next_to_the_config_file() {
        let (path, config_dir) = resolve(Some("api_key: abc\n"), &[]);
        assert_eq!(path.unwrap(), cert_in(&config_dir));
        let (path, config_dir) = resolve(None, &[]);
        assert_eq!(
            path.unwrap(),
            cert_in(&config_dir),
            "a missing config file reads as empty"
        );
    }

    #[test]
    fn ipc_cert_ignores_empty_values() {
        let (path, config_dir) = resolve(
            Some("ipc_cert_file_path: ''\nauth_token_file_path: ''\n"),
            &[
                ("DD_IPC_CERT_FILE_PATH", ""),
                ("DD_AUTH_TOKEN_FILE_PATH", ""),
            ],
        );
        assert_eq!(path.unwrap(), cert_in(&config_dir));
    }

    #[test]
    fn ipc_cert_refuses_to_guess_from_an_unparsable_config() {
        let (path, _) = resolve(Some("ipc_cert_file_path: [unterminated\n"), &[]);
        let err = path.unwrap_err();
        assert!(format!("{err:#}").contains("parsing"), "{err:#}");
    }

    #[test]
    fn ipc_cert_refuses_to_guess_from_a_secret_handle() {
        for (yaml, env, setting) in [
            (
                "ipc_cert_file_path: ENC[cert]\n",
                &[][..],
                "ipc_cert_file_path",
            ),
            (
                "auth_token_file_path: ' ENC[token]\t'\n",
                &[][..],
                "auth_token_file_path",
            ),
            (
                "api_key: abc\n",
                &[("DD_IPC_CERT_FILE_PATH", "ENC[cert]")][..],
                "DD_IPC_CERT_FILE_PATH",
            ),
            (
                "api_key: abc\n",
                &[("DD_AUTH_TOKEN_FILE_PATH", "ENC[token]")][..],
                "DD_AUTH_TOKEN_FILE_PATH",
            ),
        ] {
            let (path, _) = resolve(Some(yaml), env);
            let err = format!("{:#}", path.unwrap_err());
            assert!(err.starts_with(setting), "{err}");
            assert!(err.contains("secret handle"), "{err}");
        }
    }

    #[test]
    fn ipc_cert_skips_a_secret_handle_it_never_reaches() {
        let (path, _) = resolve(
            Some("ipc_cert_file_path: /yaml/cert.pem\nauth_token_file_path: ENC[token]\n"),
            &[],
        );
        assert_eq!(path.unwrap(), "/yaml/cert.pem");
    }

    #[test]
    fn ipc_cert_failure_is_not_cached() {
        let cache = OnceLock::new();
        let err = cached_ipc_cert_file_path(&cache, || {
            anyhow::bail!("ipc_cert_file_path is a secret handle")
        })
        .unwrap_err();
        assert!(format!("{err:#}").contains("secret handle"));
        assert!(cache.get().is_none(), "a failure must be retried");

        assert_eq!(
            cached_ipc_cert_file_path(&cache, || Ok("/etc/cert.pem".to_string())).unwrap(),
            "/etc/cert.pem"
        );
    }

    #[test]
    fn ipc_cert_success_is_cached() {
        let cache = OnceLock::new();
        assert_eq!(
            cached_ipc_cert_file_path(&cache, || Ok("/etc/cert.pem".to_string())).unwrap(),
            "/etc/cert.pem"
        );
        assert_eq!(
            cached_ipc_cert_file_path(&cache, || anyhow::bail!("must not run once cached"))
                .unwrap(),
            "/etc/cert.pem"
        );
    }

    #[test]
    fn unresolvable_ipc_cert_fails_only_where_the_placeholder_appears() {
        let fail = || anyhow::bail!("ipc_cert_file_path is a secret handle");
        let err = require_ipc_cert_file_path("${DD_PM_IPC_CERT_FILE_PATH}", fail).unwrap_err();
        assert!(format!("{err:#}").contains("secret handle"));
        let err =
            require_ipc_cert_file_path("--cert=${DD_PM_IPC_CERT_FILE_PATH}", fail).unwrap_err();
        assert!(format!("{err:#}").contains("resolving the Agent IPC certificate path"));

        // Unrelated input never asks for the path.
        for input in [
            "--executor-socket",
            "${DD_CONF_DIR}/x",
            "${DD_IPC_CERT_FILE_PATH}",
        ] {
            require_ipc_cert_file_path(input, || panic!("must not resolve for {input}")).unwrap();
        }
    }

    #[test]
    fn test_expand_vars_no_placeholder_untouched() {
        let lookup = |_: &str| Some("should-not-be-used".to_string());
        let path = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/otel-agent";
        assert_eq!(expand_vars_with(path, lookup), path);
        assert_eq!(expand_vars_with("a ${ b", lookup), "a ${ b");
    }
}
