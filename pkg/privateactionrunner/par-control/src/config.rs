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
    pub log_level: log::LevelFilter,
}

#[derive(serde::Deserialize, Default)]
struct RawConfig {
    log_level: Option<String>,
    fleet_policies_dir: Option<String>,
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
        let fleet_policies_dir =
            env_string(env, "DD_FLEET_POLICIES_DIR", raw.fleet_policies_dir.clone())
                .or_else(crate::platform::fleet_policies_dir);
        let fleet = fleet_policies_dir
            .as_deref()
            .map(read_fleet_policy)
            .transpose()?
            .flatten()
            .unwrap_or_default();

        let par = raw.private_action_runner.unwrap_or_default();
        let fleet_par = fleet.private_action_runner.unwrap_or_default();
        let enabled = match fleet_par.enabled {
            Some(value) => value,
            None => env_bool(env, "DD_PRIVATE_ACTION_RUNNER_ENABLED", par.enabled)?,
        };
        let split_enabled = match fleet_par.split_enabled {
            Some(value) => value,
            None => env_bool(
                env,
                "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
                par.split_enabled,
            )?,
        };
        let log_level = fleet
            .log_level
            .or_else(|| env_string(env, "DD_LOG_LEVEL", raw.log_level));

        Ok(Self {
            split_mode: enabled && split_enabled,
            log_level: log_level
                .as_deref()
                .map_or(log::LevelFilter::Info, parse_log_level),
        })
    }
}

/// Load the Fleet Automation policy layered over the local configuration.
/// A missing policy file is equivalent to an empty policy, matching the Agent
/// config component's `MergeFleetPolicy` behavior.
fn read_fleet_policy(dir: &str) -> Result<Option<RawConfig>> {
    if dir.is_empty() {
        return Ok(None);
    }

    let path = Path::new(dir).join("datadog.yaml");
    let contents = match std::fs::read_to_string(&path) {
        Ok(contents) => contents,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => {
            return Err(error)
                .with_context(|| format!("failed to read fleet policy: {}", path.display()));
        }
    };
    serde_yaml::from_str(&contents)
        .with_context(|| format!("failed to parse fleet policy: {}", path.display()))
        .map(Some)
}

/// A non-empty environment value wins over the YAML value.
fn env_string(env: EnvLookup<'_>, name: &str, yaml_value: Option<String>) -> Option<String> {
    env(name).filter(|value| !value.is_empty()).or(yaml_value)
}

fn env_bool(env: EnvLookup<'_>, name: &str, yaml_value: Option<bool>) -> Result<bool> {
    let Some(raw) = env_string(env, name, None) else {
        return Ok(yaml_value.unwrap_or(false));
    };
    match raw.trim() {
        "1" | "t" | "T" | "TRUE" | "true" | "True" => Ok(true),
        "0" | "f" | "F" | "FALSE" | "false" | "False" => Ok(false),
        _ => bail!("invalid boolean value for {name}: {raw:?}"),
    }
}

fn parse_log_level(raw: &str) -> log::LevelFilter {
    match raw.trim().to_ascii_lowercase().as_str() {
        "trace" => log::LevelFilter::Trace,
        "debug" => log::LevelFilter::Debug,
        "warn" | "warning" => log::LevelFilter::Warn,
        "error" | "critical" => log::LevelFilter::Error,
        "off" => log::LevelFilter::Off,
        _ => log::LevelFilter::Info,
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
        assert_eq!(config.log_level, log::LevelFilter::Trace);
    }

    #[test]
    fn fleet_policy_overrides_yaml_and_environment() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(
            dir.path().join("datadog.yaml"),
            "log_level: warning\nprivate_action_runner:\n  enabled: true\n  split_enabled: true\n",
        )
        .unwrap();
        let fleet_dir = dir.path().to_str().unwrap();

        let config = parse(
            "log_level: info\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            &[
                ("DD_FLEET_POLICIES_DIR", fleet_dir),
                ("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false"),
                ("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "false"),
                ("DD_LOG_LEVEL", "debug"),
            ],
        )
        .unwrap();

        assert!(config.split_mode);
        assert_eq!(config.log_level, log::LevelFilter::Warn);
    }

    #[test]
    fn falls_back_to_defaults() {
        let config = parse("", &[]).unwrap();
        assert!(!config.split_mode);
        assert_eq!(config.log_level, log::LevelFilter::Info);
    }

    #[test]
    fn supports_agent_log_levels() {
        for (raw, expected) in [
            ("trace", log::LevelFilter::Trace),
            ("debug", log::LevelFilter::Debug),
            ("info", log::LevelFilter::Info),
            ("warn", log::LevelFilter::Warn),
            ("warning", log::LevelFilter::Warn),
            ("error", log::LevelFilter::Error),
            ("critical", log::LevelFilter::Error),
            ("off", log::LevelFilter::Off),
        ] {
            let config = parse(&format!("log_level: {raw}\n"), &[]).unwrap();
            assert_eq!(config.log_level, expected, "log level {raw}");
        }
    }

    #[test]
    fn supports_agent_boolean_environment_values() {
        for (raw, expected) in [
            ("1", true),
            ("t", true),
            ("T", true),
            ("TRUE", true),
            ("true", true),
            ("True", true),
            ("0", false),
            ("f", false),
            ("F", false),
            ("FALSE", false),
            ("false", false),
            ("False", false),
        ] {
            let config = parse(
                "",
                &[
                    ("DD_PRIVATE_ACTION_RUNNER_ENABLED", raw),
                    ("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "true"),
                ],
            )
            .unwrap();
            assert_eq!(config.split_mode, expected, "boolean value {raw:?}");
        }
    }

    #[test]
    fn rejects_invalid_boolean_environment_value() {
        let err = parse("", &[("DD_PRIVATE_ACTION_RUNNER_ENABLED", "yes")]).unwrap_err();
        assert!(err.to_string().contains("DD_PRIVATE_ACTION_RUNNER_ENABLED"));
    }
}
