// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Optional `condition_config_any` gates for processes.d definitions.
//!
//! Mirrors the Windows legacy SCM startup checks in
//! `cmd/agent/subcommands/run/dependent_services_windows.go`: start only when any
//! configured key in a YAML file evaluates to true.

use serde::Deserialize;
use std::str::FromStr;

/// A YAML file and dotted config keys; any key set to true satisfies the gate.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct ConditionConfigFile {
    pub path: String,
    #[serde(default)]
    pub keys: Vec<String>,
}

/// Returns true when `conditions` is empty or any `(path, key)` pair is enabled.
pub fn condition_config_any_met(conditions: &[ConditionConfigFile]) -> bool {
    if conditions.is_empty() {
        return true;
    }
    conditions.iter().any(|file| {
        file.keys.iter().any(|key| {
            config_key_enabled(&file.path, key).unwrap_or_else(|err| {
                log::debug!("condition_config_any: {} key {key}: {err:#}", file.path);
                false
            })
        })
    })
}

fn config_key_enabled(path: &str, key: &str) -> anyhow::Result<bool> {
    let contents =
        std::fs::read_to_string(path).map_err(|err| anyhow::anyhow!("read {path}: {err}"))?;
    let root: serde_yaml::Value =
        serde_yaml::from_str(&contents).map_err(|err| anyhow::anyhow!("parse {path}: {err}"))?;
    let value = lookup_dotted_key(&root, key)
        .ok_or_else(|| anyhow::anyhow!("key {key} not found in {path}"))?;
    value_as_bool(value).ok_or_else(|| anyhow::anyhow!("key {key} is not a bool"))
}

fn lookup_dotted_key<'a>(root: &'a serde_yaml::Value, key: &str) -> Option<&'a serde_yaml::Value> {
    let mut current = root;
    for segment in key.split('.') {
        current = current.get(segment)?;
    }
    Some(current)
}

fn value_as_bool(value: &serde_yaml::Value) -> Option<bool> {
    match value {
        serde_yaml::Value::Bool(enabled) => Some(*enabled),
        serde_yaml::Value::Number(number) => number.as_i64().map(|n| n != 0),
        serde_yaml::Value::String(text) => parse_bool_string(text),
        _ => None,
    }
}

fn parse_bool_string(text: &str) -> Option<bool> {
    match text.trim().to_ascii_lowercase().as_str() {
        "" | "0" | "false" | "no" | "n" | "off" | "disabled" => Some(false),
        "1" | "true" | "yes" | "y" | "on" => Some(true),
        _ => bool::from_str(text).ok(),
    }
}

/// Human-readable path for logs when a config gate blocks startup.
pub fn condition_config_summary(conditions: &[ConditionConfigFile]) -> String {
    conditions
        .iter()
        .flat_map(|file| {
            file.keys
                .iter()
                .map(move |key| format!("{}:{}", file.path, key))
        })
        .collect::<Vec<_>>()
        .join(", ")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use std::path::Path;

    fn write_config(dir: &Path, name: &str, body: &str) -> String {
        let path = dir.join(name);
        let mut file = std::fs::File::create(&path).unwrap();
        file.write_all(body.as_bytes()).unwrap();
        path.to_string_lossy().into_owned()
    }

    #[test]
    fn empty_conditions_are_met() {
        assert!(condition_config_any_met(&[]));
    }

    #[test]
    fn any_matching_key_enables_gate() {
        let dir = tempfile::tempdir().unwrap();
        let agent = write_config(
            dir.path(),
            "datadog.yaml",
            "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: true\n",
        );
        let conditions = vec![ConditionConfigFile {
            path: agent,
            keys: vec![
                "process_config.process_collection.enabled".into(),
                "process_config.process_discovery.enabled".into(),
            ],
        }];
        assert!(condition_config_any_met(&conditions));
    }

    #[test]
    fn all_false_keys_block_gate() {
        let dir = tempfile::tempdir().unwrap();
        let agent = write_config(
            dir.path(),
            "datadog.yaml",
            "process_config:\n  enabled: disabled\n  process_collection:\n    enabled: false\n",
        );
        let conditions = vec![ConditionConfigFile {
            path: agent,
            keys: vec![
                "process_config.enabled".into(),
                "process_config.process_collection.enabled".into(),
            ],
        }];
        assert!(!condition_config_any_met(&conditions));
    }

    #[test]
    fn missing_file_blocks_gate() {
        let conditions = vec![ConditionConfigFile {
            path: "/nonexistent/datadog.yaml".into(),
            keys: vec!["process_config.enabled".into()],
        }];
        assert!(!condition_config_any_met(&conditions));
    }

    #[test]
    fn value_as_bool_handles_strings() {
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("disabled".into())),
            Some(false)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("true".into())),
            Some(true)
        );
    }
}
