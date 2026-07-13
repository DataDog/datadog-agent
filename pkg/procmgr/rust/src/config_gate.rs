// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Optional `condition_config_any` gates for processes.d definitions.
//!
//! Mirrors the Windows legacy SCM startup checks in
//! `cmd/agent/subcommands/run/dependent_services_windows.go`: start only when any
//! configured key evaluates to true. Resolution order matches agent config:
//! environment override, explicit YAML value, then agent default.

use serde::Deserialize;
use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::str::FromStr;

/// A YAML file and dotted config keys; any key set to true satisfies the gate.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct ConditionConfigFile {
    pub path: String,
    #[serde(default)]
    pub keys: Vec<String>,
}

struct GatedKeySpec {
    key: &'static str,
    default: bool,
    env_vars: &'static [&'static str],
}

/// Single source of truth for gated keys (mirrors `pkg/config/setup/process_settings.go`
/// and `pkg/config/setup/system_probe.go`).
const GATED_KEY_SPECS: &[GatedKeySpec] = &[
    GatedKeySpec {
        key: "process_config.enabled",
        default: false,
        env_vars: &["DD_PROCESS_CONFIG_ENABLED", "DD_PROCESS_AGENT_ENABLED"],
    },
    GatedKeySpec {
        key: "process_config.process_collection.enabled",
        default: false,
        env_vars: &[
            "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
            "DD_PROCESS_AGENT_PROCESS_COLLECTION_ENABLED",
        ],
    },
    GatedKeySpec {
        key: "process_config.container_collection.enabled",
        default: true,
        env_vars: &[
            "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED",
            "DD_PROCESS_AGENT_CONTAINER_COLLECTION_ENABLED",
        ],
    },
    GatedKeySpec {
        key: "process_config.process_discovery.enabled",
        default: true,
        env_vars: &[
            "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
            "DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
            "DD_PROCESS_CONFIG_DISCOVERY_ENABLED",
            "DD_PROCESS_AGENT_DISCOVERY_ENABLED",
        ],
    },
    GatedKeySpec {
        key: "network_config.enabled",
        default: false,
        env_vars: &["DD_SYSTEM_PROBE_NETWORK_ENABLED"],
    },
    GatedKeySpec {
        key: "system_probe_config.enabled",
        default: false,
        env_vars: &["DD_SYSTEM_PROBE_ENABLED"],
    },
];

impl GatedKeySpec {
    fn enabled(&self, path: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
        if let Some(enabled) = self.env_override() {
            return Ok(enabled);
        }
        if let Some(enabled) = yaml.optional_bool_key(path, self.key)? {
            return Ok(enabled);
        }
        Ok(self.default)
    }

    fn env_override(&self) -> Option<bool> {
        self.env_vars
            .iter()
            .filter_map(|name| std::env::var(name).ok())
            .find_map(|value| parse_bool_string(&value))
    }
}

struct YamlCache(HashMap<String, serde_yaml::Value>);

impl YamlCache {
    fn load(&mut self, path: &str) -> anyhow::Result<&serde_yaml::Value> {
        match self.0.entry(path.to_owned()) {
            Entry::Occupied(entry) => Ok(entry.into_mut()),
            Entry::Vacant(entry) => {
                let contents = std::fs::read_to_string(path)
                    .map_err(|err| anyhow::anyhow!("read {path}: {err}"))?;
                let root = serde_yaml::from_str(&contents)
                    .map_err(|err| anyhow::anyhow!("parse {path}: {err}"))?;
                Ok(entry.insert(root))
            }
        }
    }

    fn optional_bool_key(&mut self, path: &str, key: &str) -> anyhow::Result<Option<bool>> {
        match lookup_dotted_key(self.load(path)?, key) {
            Some(value) => value_as_bool(value)
                .ok_or_else(|| anyhow::anyhow!("key {key} is not a bool"))
                .map(Some),
            None => Ok(None),
        }
    }

    #[cfg(test)]
    fn loaded_file_count(&self) -> usize {
        self.0.len()
    }
}

/// Returns true when `conditions` is empty or any `(path, key)` pair is enabled.
pub fn condition_config_any_met(conditions: &[ConditionConfigFile]) -> bool {
    if conditions.is_empty() {
        return true;
    }

    let mut yaml = YamlCache(HashMap::new());
    conditions.iter().any(|file| {
        file.keys.iter().any(|key| {
            config_key_enabled(&file.path, key, &mut yaml).unwrap_or_else(|err| {
                log::debug!("condition_config_any: {} key {key}: {err:#}", file.path);
                false
            })
        })
    })
}

fn config_key_enabled(path: &str, key: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
    GATED_KEY_SPECS
        .iter()
        .find(|spec| spec.key == key)
        .ok_or_else(|| anyhow::anyhow!("unknown config key {key}"))?
        .enabled(path, yaml)
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
    use std::sync::Mutex;

    static ENV_TEST_LOCK: Mutex<()> = Mutex::new(());

    fn write_config(dir: &Path, name: &str, body: &str) -> String {
        let path = dir.join(name);
        let mut file = std::fs::File::create(&path).unwrap();
        file.write_all(body.as_bytes()).unwrap();
        path.to_string_lossy().into_owned()
    }

    fn process_agent_conditions(agent_path: String) -> Vec<ConditionConfigFile> {
        vec![ConditionConfigFile {
            path: agent_path,
            keys: vec![
                "process_config.enabled".into(),
                "process_config.process_collection.enabled".into(),
                "process_config.container_collection.enabled".into(),
                "process_config.process_discovery.enabled".into(),
            ],
        }]
    }

    fn with_env_lock<F: FnOnce()>(test: F) {
        let _lock = ENV_TEST_LOCK.lock().unwrap_or_else(|err| err.into_inner());
        test();
    }

    fn clear_gated_env_vars() {
        for spec in GATED_KEY_SPECS {
            for env_name in spec.env_vars {
                // SAFETY: callers must hold ENV_TEST_LOCK.
                unsafe { std::env::remove_var(env_name) };
            }
        }
    }

    struct EnvGuard {
        name: &'static str,
        previous: Option<String>,
    }

    impl EnvGuard {
        fn set(name: &'static str, value: &str) -> Self {
            let previous = std::env::var(name).ok();
            // SAFETY: callers must hold ENV_TEST_LOCK.
            unsafe { std::env::set_var(name, value) };
            Self { name, previous }
        }
    }

    impl Drop for EnvGuard {
        fn drop(&mut self) {
            match &self.previous {
                Some(value) => unsafe { std::env::set_var(self.name, value) },
                None => unsafe { std::env::remove_var(self.name) },
            }
        }
    }

    #[test]
    fn empty_conditions_are_met() {
        assert!(condition_config_any_met(&[]));
    }

    #[test]
    fn any_matching_key_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

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
        });
    }

    #[test]
    fn stock_config_uses_agent_defaults() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn all_false_keys_block_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn env_override_can_disable_default_enabled_keys() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _collection =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            let _discovery =
                EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn env_override_can_enable_when_yaml_keys_missing() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _collection =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            let _discovery =
                EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn yaml_cache_reads_each_path_once() {
        let dir = tempfile::tempdir().unwrap();
        let path = write_config(
            dir.path(),
            "datadog.yaml",
            "process_config:\n  container_collection:\n    enabled: true\n  process_discovery:\n    enabled: false\n",
        );

        let mut cache = YamlCache(HashMap::new());
        for key in [
            "process_config.container_collection.enabled",
            "process_config.process_discovery.enabled",
        ] {
            cache.optional_bool_key(&path, key).unwrap();
        }
        assert_eq!(cache.loaded_file_count(), 1);
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
