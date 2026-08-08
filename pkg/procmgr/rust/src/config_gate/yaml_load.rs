// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! YAML loading for config gates.
//!
//! Mirrors `pkg/config/nodetreemodel/read_config_file.go`: try strict parsing first,
//! then fall back to a permissive loader that keeps the last duplicate key (matching
//! Go `yaml.Unmarshal`).
//!
//! Strict parsing uses `serde_yaml::Value` (rejects duplicate mapping keys). The
//! fallback deserializes mappings into [`HashMap`] so duplicate keys last-win, then
//! converts back to `serde_yaml::Value` for gate lookup.

use std::collections::HashMap;

use anyhow::{Context, Result};
use log::debug;
use serde::Deserialize;
use serde_yaml::{Mapping, Number, Sequence, Value};

/// Parse YAML for config-gate lookups, matching Agent config file semantics.
pub(super) fn load_yaml(contents: &str) -> Result<Value> {
    match serde_yaml::from_str(contents) {
        Ok(value) => Ok(value),
        Err(strict_err) => {
            debug!("strict YAML parse failed, retrying permissive: {strict_err}");
            load_yaml_permissive(contents).with_context(|| strict_err.to_string())
        }
    }
}

/// Same shape as `serde_yaml::Value`, but mappings use `HashMap` (last duplicate wins).
#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum PermissiveValue {
    Bool(bool),
    Number(Number),
    String(String),
    Sequence(Vec<PermissiveValue>),
    Mapping(HashMap<String, PermissiveValue>),
}

fn load_yaml_permissive(contents: &str) -> Result<Value> {
    let root: PermissiveValue = serde_yaml::from_str(contents)?;
    Ok(permissive_to_value(root))
}

fn permissive_to_value(value: PermissiveValue) -> Value {
    match value {
        PermissiveValue::Bool(enabled) => Value::Bool(enabled),
        PermissiveValue::Number(number) => Value::Number(number),
        PermissiveValue::String(text) => Value::String(text),
        PermissiveValue::Sequence(items) => Value::Sequence(Sequence::from_iter(
            items.into_iter().map(permissive_to_value),
        )),
        PermissiveValue::Mapping(items) => {
            let mut map = Mapping::new();
            for (key, item) in items {
                map.insert(Value::String(key), permissive_to_value(item));
            }
            Value::Mapping(map)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strict_parse_used_when_no_duplicates() {
        let yaml = "process_config:\n  enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            root.get("process_config").and_then(|v| v.get("enabled")),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn permissive_parse_keeps_last_duplicate_key() {
        let yaml = "process_config:\n  enabled: false\nprocess_config:\n  enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            root.get("process_config").and_then(|v| v.get("enabled")),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn permissive_parse_nested_duplicate_key_last_wins() {
        let yaml = "process_config:\n  process_collection:\n    enabled: false\n  process_collection:\n    enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            root.get("process_config")
                .and_then(|v| v.get("process_collection"))
                .and_then(|v| v.get("enabled")),
            Some(&Value::Bool(true))
        );
    }
}
