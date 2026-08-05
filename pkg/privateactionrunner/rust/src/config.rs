// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! The datadog.yaml settings the lifecycle scaffold needs.

use anyhow::{Context, Result, bail};
use std::path::Path;

/// Lookup of `DD_*` environment overrides, injected so it can be faked in tests.
type EnvLookup<'a> = &'a dyn Fn(&str) -> Option<String>;

#[derive(Debug)]
pub struct Config {
    /// True when both `private_action_runner.enabled` and `.split_enabled` are set.
    pub split_mode: bool,
    pub log_level: log::Level,
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
}

impl Config {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        Self::from_yaml(&contents, &|name| std::env::var(name).ok())
    }

    fn from_yaml(yaml: &str, env: EnvLookup<'_>) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let par = raw.private_action_runner.unwrap_or_default();
        let enabled = env_bool(env, "DD_PRIVATE_ACTION_RUNNER_ENABLED", par.enabled)?;
        let split_enabled = env_bool(
            env,
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
            par.split_enabled,
        )?;
        Ok(Self {
            split_mode: enabled && split_enabled,
            log_level: env_string(env, "DD_LOG_LEVEL", raw.log_level)
                .as_deref()
                .map_or(log::Level::Info, parse_log_level),
        })
    }
}

/// A non-empty environment value wins over the YAML value.
fn env_string(env: EnvLookup<'_>, name: &str, yaml_value: Option<String>) -> Option<String> {
    env(name).filter(|value| !value.is_empty()).or(yaml_value)
}

fn env_bool(env: EnvLookup<'_>, name: &str, yaml_value: Option<bool>) -> Result<bool> {
    let Some(raw) = env_string(env, name, None) else {
        return Ok(yaml_value.unwrap_or(false));
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "1" | "t" | "true" | "y" | "yes" => Ok(true),
        "0" | "f" | "false" | "n" | "no" => Ok(false),
        _ => bail!("invalid boolean value for {name}: {raw:?}"),
    }
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

    fn parse(yaml: &str, env: &[(&str, &str)]) -> Result<Config> {
        Config::from_yaml(yaml, &|name| {
            env.iter()
                .find(|(key, _)| *key == name)
                .map(|(_, value)| (*value).to_string())
        })
    }

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
            let config = parse(yaml, &[]).unwrap();
            assert_eq!(config.split_mode, expected, "yaml={yaml:?}");
        }
    }

    #[test]
    fn environment_overrides_yaml() {
        let config = parse(
            "log_level: debug\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            &[
                ("DD_PRIVATE_ACTION_RUNNER_ENABLED", "true"),
                ("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "true"),
                ("DD_LOG_LEVEL", "trace"),
            ],
        )
        .unwrap();
        assert!(config.split_mode);
        assert_eq!(config.log_level, log::Level::Trace);
    }

    #[test]
    fn falls_back_to_defaults() {
        let config = parse("", &[]).unwrap();
        assert!(!config.split_mode);
        assert_eq!(config.log_level, log::Level::Info);
    }

    #[test]
    fn rejects_invalid_boolean_environment_value() {
        let err = parse("", &[("DD_PRIVATE_ACTION_RUNNER_ENABLED", "sometimes")]).unwrap_err();
        assert!(err.to_string().contains("DD_PRIVATE_ACTION_RUNNER_ENABLED"));
    }
}
