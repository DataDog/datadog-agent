// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! YAML loading for config gates.
//!
//! Primary path uses [`saphyr_parser`] so plain-scalar YAML 1.1 bools (`yes`/`on`/…)
//! are coerced at parse time, matching Go `gopkg.in/yaml.v2`. Quoted scalars stay strings.
//!
//! Duplicate mapping keys last-win (matching Go `yaml.Unmarshal`). On parse failure,
//! falls back to permissive [`serde_yaml`] with the same last-win semantics.

mod permissive;
mod saphyr;

use anyhow::{Context, Result};
use log::debug;
use serde_yaml::Value;

/// Parse YAML for config-gate lookups, matching Agent config file semantics.
pub(super) fn load_yaml(contents: &str) -> Result<Value> {
    let mut root = match saphyr::load(contents) {
        Ok(value) => value,
        Err(err) => {
            debug!("saphyr YAML parse failed, retrying serde_yaml permissive: {err}");
            permissive::load(contents).with_context(|| err.to_string())?
        }
    };
    root.apply_merge()
        .context("apply YAML merge keys for config gate lookup")?;
    Ok(root)
}

#[cfg(test)]
mod tests {
    use serde_yaml::Value;

    use super::*;

    fn dotted<'a>(root: &'a Value, path: &str) -> Option<&'a Value> {
        let mut current = root;
        for segment in path.split('.') {
            current = current.get(segment)?;
        }
        Some(current)
    }

    #[test]
    fn multi_document_stream_uses_first_document() {
        let yaml = "process_config:\n  process_collection:\n    enabled: true\n---\nprocess_config:\n  process_collection:\n    enabled: false\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            dotted(&root, "process_config.process_collection.enabled"),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn strict_parse_used_when_no_duplicates() {
        let root = load_yaml("process_config:\n  enabled: true\n").unwrap();
        assert_eq!(
            dotted(&root, "process_config.enabled"),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn permissive_parse_keeps_last_duplicate_key() {
        let yaml = "process_config:\n  enabled: false\nprocess_config:\n  enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            dotted(&root, "process_config.enabled"),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn permissive_parse_nested_duplicate_key_last_wins() {
        let yaml = "process_config:\n  process_collection:\n    enabled: false\n  process_collection:\n    enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            dotted(&root, "process_config.process_collection.enabled"),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn permissive_parse_accepts_null_with_duplicate_keys() {
        let yaml = "network_config:\n  enabled:\nsystem_probe_config:\n  enabled: true\nprocess_config:\n  enabled: false\nprocess_config:\n  process_collection:\n    enabled: true\n";
        let root = load_yaml(yaml).unwrap();
        assert_eq!(dotted(&root, "network_config.enabled"), Some(&Value::Null));
        assert_eq!(
            dotted(&root, "system_probe_config.enabled"),
            Some(&Value::Bool(true))
        );
        assert_eq!(
            dotted(&root, "process_config.process_collection.enabled"),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn merge_keys_expand_disabled_process_config_defaults() {
        let yaml = r#"
disabled: &disabled
  process_collection:
    enabled: false
  container_collection:
    enabled: false

process_config:
  <<: *disabled
"#;
        let root = load_yaml(yaml).unwrap();
        assert_eq!(
            dotted(&root, "process_config.process_collection.enabled"),
            Some(&Value::Bool(false))
        );
        assert_eq!(
            dotted(&root, "process_config.container_collection.enabled"),
            Some(&Value::Bool(false))
        );
    }
}
