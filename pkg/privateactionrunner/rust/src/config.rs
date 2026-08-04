// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Minimal launch and process-manager configuration for the lifecycle scaffold.

use anyhow::{Context, Result, bail};
use std::path::{Path, PathBuf};

#[cfg(not(windows))]
pub const DEFAULT_PROCMGR_SOCKET: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
#[cfg(windows)]
pub const DEFAULT_PROCMGR_SOCKET: &str = r"\\.\pipe\datadog-procmgrd";
pub const DEFAULT_EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    pub procmgr_socket: PathBuf,
    pub executor_process_name: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LaunchGate {
    pub split_mode: bool,
}

#[derive(serde::Deserialize, Default)]
struct RawConfig {
    log_level: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default)]
struct RawPar {
    enabled: Option<bool>,
    split_enabled: Option<bool>,
    procmgr_socket_path: Option<String>,
    executor_process_name: Option<String>,
}

impl Config {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        let raw = read_raw_config(path)?;
        let par = raw.private_action_runner.unwrap_or_default();
        Ok(Self {
            procmgr_socket: std::env::var("DD_PRIVATE_ACTION_RUNNER_PROCMGR_SOCKET_PATH")
                .ok()
                .filter(|value| !value.is_empty())
                .or(par.procmgr_socket_path)
                .map(PathBuf::from)
                .unwrap_or_else(|| PathBuf::from(DEFAULT_PROCMGR_SOCKET)),
            executor_process_name: std::env::var("DD_PRIVATE_ACTION_RUNNER_EXECUTOR_PROCESS_NAME")
                .ok()
                .filter(|value| !value.is_empty())
                .or(par.executor_process_name)
                .unwrap_or_else(|| DEFAULT_EXECUTOR_PROCESS_NAME.to_string()),
        })
    }
}

impl LaunchGate {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        let raw = read_raw_config(path)?;
        Self::from_raw_with_env(raw, |name| std::env::var(name).ok())
    }

    #[cfg(test)]
    fn from_yaml_str_with_env(yaml: &str, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        Self::from_raw_with_env(raw, env)
    }

    fn from_raw_with_env(raw: RawConfig, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let par = raw.private_action_runner.unwrap_or_default();
        let enabled = env_bool_override(par.enabled, "DD_PRIVATE_ACTION_RUNNER_ENABLED", &env)?;
        let split_enabled = env_bool_override(
            par.split_enabled,
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
            &env,
        )?;
        Ok(Self {
            split_mode: enabled.unwrap_or(false) && split_enabled.unwrap_or(false),
        })
    }
}

pub fn log_level_from_yaml_file(path: &Path) -> log::Level {
    std::env::var("DD_LOG_LEVEL")
        .ok()
        .or_else(|| read_raw_config(path).ok().and_then(|raw| raw.log_level))
        .as_deref()
        .map(parse_log_level)
        .unwrap_or(log::Level::Info)
}

fn read_raw_config(path: &Path) -> Result<RawConfig> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("failed to read config file: {}", path.display()))?;
    serde_yaml::from_str(&contents).context("failed to parse datadog.yaml")
}

fn env_bool_override(
    yaml_value: Option<bool>,
    name: &str,
    env: &impl Fn(&str) -> Option<String>,
) -> Result<Option<bool>> {
    let Some(raw) = env(name) else {
        return Ok(yaml_value);
    };
    let value = match raw.trim().to_ascii_lowercase().as_str() {
        "1" | "t" | "true" | "y" | "yes" => true,
        "0" | "f" | "false" | "n" | "no" => false,
        _ => bail!("invalid boolean value for {name}: {raw:?}"),
    };
    Ok(Some(value))
}

fn parse_log_level(raw: &str) -> log::Level {
    match raw.trim().to_ascii_lowercase().as_str() {
        "trace" => log::Level::Trace,
        "debug" => log::Level::Debug,
        "warn" | "warning" => log::Level::Warn,
        "error" | "critical" | "off" => log::Level::Error,
        _ => log::Level::Info,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_mode_requires_both_switches() {
        for (yaml, expected) in [
            ("private_action_runner:\n  split_enabled: true\n", false),
            (
                "private_action_runner:\n  enabled: true\n  split_enabled: true\n",
                true,
            ),
            (
                "private_action_runner:\n  enabled: false\n  split_enabled: true\n",
                false,
            ),
        ] {
            let gate = LaunchGate::from_yaml_str_with_env(yaml, |_| None).unwrap();
            assert_eq!(gate.split_mode, expected, "yaml={yaml:?}");
        }
    }

    #[test]
    fn environment_overrides_activation_gate() {
        let gate = LaunchGate::from_yaml_str_with_env(
            "private_action_runner:\n  enabled: false\n  split_enabled: false\n",
            |name| match name {
                "DD_PRIVATE_ACTION_RUNNER_ENABLED" | "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => {
                    Some("true".to_string())
                }
                _ => None,
            },
        )
        .unwrap();
        assert!(gate.split_mode);
    }

    #[test]
    fn rejects_invalid_boolean_environment_value() {
        let err = LaunchGate::from_yaml_str_with_env("", |name| {
            (name == "DD_PRIVATE_ACTION_RUNNER_ENABLED").then(|| "sometimes".to_string())
        })
        .unwrap_err();
        assert!(err.to_string().contains("DD_PRIVATE_ACTION_RUNNER_ENABLED"));
    }
}
