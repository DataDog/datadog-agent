// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Launch-time configuration for par-control.

use anyhow::{Context, Result, bail};
use std::path::Path;

#[derive(serde::Deserialize, Default, Clone)]
struct RawConfig {
    log_level: Option<String>,
    fleet_policies_dir: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawPar {
    enabled: Option<bool>,
    split_enabled: Option<bool>,
    self_enroll: Option<bool>,
}

/// Settings needed before identity bootstrap.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LaunchGate {
    pub split_mode: bool,
    pub self_enroll: bool,
}

/// Log level and launch gate, resolved in one pass over `datadog.yaml` and the
/// fleet policy overlay.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Launch {
    pub log_level: log::LevelFilter,
    pub gate: LaunchGate,
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

impl Launch {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        Self::from_yaml_str_with_env(&contents, |name| std::env::var(name).ok())
    }

    #[cfg(test)]
    fn from_yaml_str(yaml: &str) -> Result<Self> {
        Self::from_yaml_str_with_env(yaml, |_| None)
    }

    fn from_yaml_str_with_env(yaml: &str, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let fleet_dir = env("DD_FLEET_POLICIES_DIR")
            .filter(|value| !value.is_empty())
            .or(raw.fleet_policies_dir)
            .or_else(crate::platform::fleet_policies_dir);
        let fleet = fleet_dir
            .as_deref()
            .map(read_fleet_policy)
            .transpose()?
            .flatten()
            .unwrap_or_default();

        let log_level = fleet
            .log_level
            .or_else(|| env("DD_LOG_LEVEL").filter(|value| !value.is_empty()))
            .or(raw.log_level)
            .as_deref()
            .map(parse_log_level)
            .unwrap_or(log::LevelFilter::Info);

        let par = raw.private_action_runner.unwrap_or_default();
        let fleet_par = fleet.private_action_runner.unwrap_or_default();
        let enabled = resolve_bool(
            fleet_par.enabled,
            par.enabled,
            "DD_PRIVATE_ACTION_RUNNER_ENABLED",
            &env,
        )?;
        let split_enabled = resolve_bool(
            fleet_par.split_enabled,
            par.split_enabled,
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
            &env,
        )?;
        let self_enroll = resolve_bool(
            fleet_par.self_enroll,
            par.self_enroll,
            "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL",
            &env,
        )?;

        Ok(Self {
            log_level,
            gate: LaunchGate {
                split_mode: enabled.unwrap_or(false) && split_enabled.unwrap_or(false),
                self_enroll: self_enroll.unwrap_or(true),
            },
        })
    }
}

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

/// Precedence: fleet policy > environment > local YAML.
fn resolve_bool(
    fleet_value: Option<bool>,
    yaml_value: Option<bool>,
    name: &str,
    env: &impl Fn(&str) -> Option<String>,
) -> Result<Option<bool>> {
    if fleet_value.is_some() {
        return Ok(fleet_value);
    }
    let Some(raw) = env(name).filter(|value| !value.is_empty()) else {
        return Ok(yaml_value);
    };
    let value = match raw.trim() {
        "1" | "t" | "T" | "TRUE" | "true" | "True" => true,
        "0" | "f" | "F" | "FALSE" | "false" | "False" => false,
        _ => bail!("invalid boolean value for {name}: {raw:?}"),
    };
    Ok(Some(value))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_mode_requires_enabled_and_split_enabled() {
        let cases = [
            ("", false),
            ("private_action_runner:\n  enabled: true\n", false),
            ("private_action_runner:\n  split_enabled: true\n", false),
            (
                "private_action_runner:\n  enabled: true\n  split_enabled: true\n",
                true,
            ),
            (
                "private_action_runner:\n  enabled: false\n  split_enabled: true\n",
                false,
            ),
        ];
        for (yaml, want) in cases {
            let launch = Launch::from_yaml_str(yaml).unwrap();
            assert_eq!(launch.gate.split_mode, want, "yaml: {yaml:?}");
        }
    }

    #[test]
    fn launch_gate_environment_overrides_yaml() {
        let env = |name: &str| match name {
            "DD_PRIVATE_ACTION_RUNNER_ENABLED" => Some("true".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => Some("1".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL" => Some("false".to_string()),
            "DD_LOG_LEVEL" => Some("trace".to_string()),
            _ => None,
        };
        let launch = Launch::from_yaml_str_with_env(
            "log_level: warn\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            env,
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Trace);
    }

    #[test]
    fn empty_environment_overrides_fall_back_to_yaml() {
        let env = |name: &str| match name {
            "DD_PRIVATE_ACTION_RUNNER_ENABLED"
            | "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"
            | "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL" => Some(String::new()),
            _ => None,
        };
        let launch = Launch::from_yaml_str_with_env(
            "private_action_runner:\n  enabled: true\n  split_enabled: true\n  self_enroll: false\n",
            env,
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
    }

    #[test]
    fn fleet_policy_overrides_local_config_and_environment() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(
            dir.path().join("datadog.yaml"),
            "log_level: error\nprivate_action_runner:\n  enabled: true\n  split_enabled: true\n  self_enroll: false\n",
        )
        .unwrap();
        let fleet_dir = dir.path().to_string_lossy().into_owned();
        let launch = Launch::from_yaml_str_with_env(
            "log_level: debug\nprivate_action_runner:\n  enabled: false\n  split_enabled: false\n",
            |name| match name {
                "DD_FLEET_POLICIES_DIR" => Some(fleet_dir.clone()),
                "DD_PRIVATE_ACTION_RUNNER_ENABLED" | "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => {
                    Some("false".to_string())
                }
                "DD_LOG_LEVEL" => Some("trace".to_string()),
                _ => None,
            },
        )
        .unwrap();
        assert!(launch.gate.split_mode);
        assert!(!launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Error);
    }

    #[test]
    fn launch_gate_accepts_agent_boolean_environment_values() {
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
            let launch = Launch::from_yaml_str_with_env("", |name| match name {
                "DD_PRIVATE_ACTION_RUNNER_ENABLED" => Some(raw.to_string()),
                "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => Some("true".to_string()),
                _ => None,
            })
            .unwrap();
            assert_eq!(launch.gate.split_mode, expected, "boolean value {raw:?}");
        }
    }

    #[test]
    fn launch_gate_rejects_invalid_environment_boolean() {
        let result = Launch::from_yaml_str_with_env("", |name| {
            (name == "DD_PRIVATE_ACTION_RUNNER_ENABLED").then(|| "sometimes".to_string())
        });
        assert!(result.is_err());
    }

    #[test]
    fn gate_resolves_without_identity() {
        let launch = Launch::from_yaml_str("site: datadoghq.com\n").unwrap();
        assert!(!launch.gate.split_mode);
        assert!(launch.gate.self_enroll);
        assert_eq!(launch.log_level, log::LevelFilter::Info);
    }

    #[test]
    fn gate_honors_explicit_self_enroll_false() {
        let launch =
            Launch::from_yaml_str("private_action_runner:\n  self_enroll: false\n").unwrap();
        assert!(!launch.gate.self_enroll);
    }

    #[test]
    fn parses_agent_log_levels() {
        for (raw, want) in [
            ("debug", log::LevelFilter::Debug),
            ("TRACE", log::LevelFilter::Trace),
            ("warn", log::LevelFilter::Warn),
            ("warning", log::LevelFilter::Warn),
            ("error", log::LevelFilter::Error),
            ("critical", log::LevelFilter::Error),
            ("off", log::LevelFilter::Off),
            ("not-a-level", log::LevelFilter::Info),
        ] {
            let launch = Launch::from_yaml_str(&format!("log_level: {raw}\n")).unwrap();
            assert_eq!(launch.log_level, want, "log_level: {raw}");
        }
    }

    #[test]
    fn reports_a_missing_config_file() {
        let dir = tempfile::tempdir().unwrap();
        let error = Launch::from_yaml_file(&dir.path().join("missing.yaml")).unwrap_err();
        assert!(
            format!("{error:#}").contains("failed to read config file"),
            "{error:#}"
        );
    }
}
