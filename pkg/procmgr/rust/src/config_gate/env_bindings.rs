// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Config-key to environment-variable bindings for procmgr config gates.
//!
//! Source of truth: `env_vars` on each setting in `pkg/config/schema/yaml/`, merged by
//! `dda inv schema.codegen` into the generated Go init settings. Keys listed in
//! [`ENV_BINDINGS`] use **only** the named variables; every other key uses the Agent
//! convention `DD_<KEY_WITH_UNDERSCORES>`. The table covers only the keys config gates
//! evaluate; the `env_bindings_match_config_schema` unit test fails when it drifts
//! from the schema.
//!
//! Each name is resolved first against the Agent service environment (the Windows SCM
//! `Environment` value, see [`crate::platform::agent_service_env_var`]) and then against
//! dd-procmgr's own environment, so a service-local override is visible to gates exactly
//! as it is to the Agent.
//!
//! The two lookups reconstruct one environment rather than forming a fallback chain. The
//! SCM merges its block over the inherited environment before the Agent starts, so an
//! entry present there shadows dd-procmgr's own value even when it is empty. Only after
//! that merge does the Agent's rule apply: `os.LookupEnv(..); ok && value != ""` in
//! `pkg/config/nodetreemodel/config.go`, which reads an empty value as unset and falls
//! through to the config file and then the schema default.

use crate::agent_yaml::parse_bool_string;

struct EnvBinding {
    key: &'static str,
    env_vars: &'static [&'static str],
}

/// Non-default env bindings. Keys omitted here resolve via `DD_<KEY>`.
const ENV_BINDINGS: &[EnvBinding] = &[
    // process.go / process_settings.go
    EnvBinding {
        key: "process_config.enabled",
        env_vars: &["DD_PROCESS_CONFIG_ENABLED", "DD_PROCESS_AGENT_ENABLED"],
    },
    EnvBinding {
        key: "process_config.process_collection.enabled",
        env_vars: &[
            "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
            "DD_PROCESS_AGENT_PROCESS_COLLECTION_ENABLED",
        ],
    },
    EnvBinding {
        key: "process_config.container_collection.enabled",
        env_vars: &[
            "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED",
            "DD_PROCESS_AGENT_CONTAINER_COLLECTION_ENABLED",
        ],
    },
    EnvBinding {
        key: "process_config.process_discovery.enabled",
        env_vars: &[
            "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
            "DD_PROCESS_AGENT_PROCESS_DISCOVERY_ENABLED",
            "DD_PROCESS_CONFIG_DISCOVERY_ENABLED",
            "DD_PROCESS_AGENT_DISCOVERY_ENABLED",
        ],
    },
    // system_probe_settings.go
    EnvBinding {
        key: "network_config.enabled",
        env_vars: &["DD_SYSTEM_PROBE_NETWORK_ENABLED"],
    },
    EnvBinding {
        key: "system_probe_config.enabled",
        env_vars: &["DD_SYSTEM_PROBE_ENABLED"],
    },
    EnvBinding {
        key: "service_monitoring_config.enabled",
        env_vars: &["DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED"],
    },
    EnvBinding {
        key: "system_probe_config.enable_co_re",
        env_vars: &["DD_ENABLE_CO_RE"],
    },
    EnvBinding {
        key: "network_config.enable_ringbuffers",
        env_vars: &["DD_SYSTEM_PROBE_NETWORK_ENABLE_RINGBUFFERS"],
    },
    EnvBinding {
        key: "network_config.enable_ebpfless",
        env_vars: &["DD_ENABLE_EBPFLESS", "DD_NETWORK_CONFIG_ENABLE_EBPFLESS"],
    },
    EnvBinding {
        key: "system_probe_config.process_config.enabled",
        env_vars: &["DD_SYSTEM_PROBE_PROCESS_ENABLED"],
    },
    EnvBinding {
        key: "dynamic_instrumentation.enabled",
        env_vars: &["DD_DYNAMIC_INSTRUMENTATION_ENABLED"],
    },
    // common_settings.go
    EnvBinding {
        key: "infrastructure_mode",
        env_vars: &["DD_INFRASTRUCTURE_MODE"],
    },
];

/// First non-empty value among the variables bound to `key`.
pub(super) fn env_string_for_key(key: &str) -> Option<String> {
    match env_vars_for_key(key) {
        [] => env_var_value(&auto_env_var_for_key(key)),
        names => names.iter().find_map(|name| env_var_value(name)),
    }
}

pub(super) fn env_bool_for_key(key: &str) -> Option<bool> {
    env_string_for_key(key).map(|value| parse_bool_string(&value).unwrap_or(false))
}

/// Whether any variable bound to `key` is set (mirrors the env source of Go `IsConfigured`).
pub(super) fn env_configured_for_key(key: &str) -> bool {
    env_string_for_key(key).is_some()
}

/// Agent service environment first, then dd-procmgr's own environment.
///
/// A service entry shadows the process environment even when empty, so the empty check
/// runs on the winning entry rather than on each source in turn.
pub(super) fn env_var_value(name: &str) -> Option<String> {
    if let Some(value) = agent_service_env_var(name) {
        return Some(value).filter(|value| !value.is_empty());
    }
    std::env::var(name).ok().filter(|value| !value.is_empty())
}

fn env_vars_for_key(key: &str) -> &'static [&'static str] {
    ENV_BINDINGS
        .iter()
        .find(|binding| binding.key == key)
        .map_or(&[], |binding| binding.env_vars)
}

fn auto_env_var_for_key(key: &str) -> String {
    format!("DD_{}", key.replace('.', "_").to_uppercase())
}

fn agent_service_env_var(name: &str) -> Option<String> {
    #[cfg(any(test, feature = "test-helpers"))]
    if let Some(vars) = test_agent_service_env() {
        // Presence is preserved here, as in the real SCM read: an empty entry shadows
        // the process environment instead of falling through to it.
        return vars
            .iter()
            .find(|(key, _)| key.eq_ignore_ascii_case(name))
            .map(|(_, value)| value.clone());
    }

    crate::platform::agent_service_env_var(name)
}

/// Gate inputs [`ENV_BINDINGS`] does not cover: the generated name for
/// `discovery.enabled`, the fleet policy directory, the ECS Fargate probe behind the
/// `discovery.enabled` platform default, and `DD_CONF_DIR`, which shipped templates
/// expand inside the gated path itself.
#[cfg(any(test, feature = "test-helpers"))]
const UNBOUND_GATE_ENV_VARS: &[&str] = &[
    "DD_DISCOVERY_ENABLED",
    "DD_FLEET_POLICIES_DIR",
    "DD_CONF_DIR",
    "ECS_FARGATE",
    "AWS_EXECUTION_ENV",
];

/// Every environment variable that can influence gate resolution, for harnesses that
/// need a hermetic daemon.
///
/// Derived from [`ENV_BINDINGS`] rather than hardcoded, so adding a binding covers it
/// here too. A harness that misses one gets a gate flipped by the runner's environment,
/// which surfaces as a test failing on one machine only.
///
/// The system-probe module knobs behind the derived `system_probe_config.enabled` are
/// not included: they resolve through generated names off a key list that lives in
/// `super::system_probe`, and only a gate on that one key reaches them.
#[cfg(any(test, feature = "test-helpers"))]
pub fn gate_env_var_names() -> Vec<String> {
    ENV_BINDINGS
        .iter()
        .flat_map(|binding| {
            binding
                .env_vars
                .iter()
                .map(|name| (*name).to_owned())
                .chain(std::iter::once(auto_env_var_for_key(binding.key)))
        })
        .chain(UNBOUND_GATE_ENV_VARS.iter().map(|name| (*name).to_owned()))
        .collect()
}

#[cfg(any(test, feature = "test-helpers"))]
static TEST_AGENT_SERVICE_ENV: std::sync::Mutex<Option<std::collections::HashMap<String, String>>> =
    std::sync::Mutex::new(None);

/// Replaces the Agent service environment lookup for tests; `None` restores the real one.
///
/// Lives here rather than in the Windows platform module so that the lookup order and
/// the gate decisions that depend on it are exercised on every platform.
#[cfg(any(test, feature = "test-helpers"))]
pub fn set_test_agent_service_env(vars: Option<std::collections::HashMap<String, String>>) {
    *TEST_AGENT_SERVICE_ENV
        .lock()
        .unwrap_or_else(|err| err.into_inner()) = vars;
}

#[cfg(any(test, feature = "test-helpers"))]
fn test_agent_service_env() -> Option<std::collections::HashMap<String, String>> {
    TEST_AGENT_SERVICE_ENV
        .lock()
        .unwrap_or_else(|err| err.into_inner())
        .clone()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_yaml::Value;
    use std::collections::HashMap;
    use std::path::{Path, PathBuf};

    /// The property the e2e env scrub depends on: a binding added to the table is
    /// scrubbed without anyone remembering to update the harness.
    #[test]
    fn gate_env_var_names_covers_every_binding() {
        let names = gate_env_var_names();

        for binding in ENV_BINDINGS {
            for var in binding.env_vars {
                assert!(
                    names.iter().any(|name| name == var),
                    "{var} missing from gate_env_var_names"
                );
            }
            let generated = auto_env_var_for_key(binding.key);
            assert!(
                names.contains(&generated),
                "{generated} missing from gate_env_var_names"
            );
        }

        for var in [
            "DD_DISCOVERY_ENABLED",
            "DD_FLEET_POLICIES_DIR",
            "DD_CONF_DIR",
            "ECS_FARGATE",
            "AWS_EXECUTION_ENV",
        ] {
            assert!(
                names.iter().any(|name| name == var),
                "{var} missing from gate_env_var_names"
            );
        }
    }

    /// Every key in [`ENV_BINDINGS`] must list the same `env_vars` the Agent schema
    /// declares (or the auto `DD_<KEY>` name when the schema omits `env_vars`).
    ///
    /// Schema is the source of truth via `dda inv schema.codegen`; this catches a
    /// renamed or reordered binding without regenerating Rust.
    #[test]
    fn env_bindings_match_config_schema() {
        let schema = schema_env_vars();
        let mut mismatches = Vec::new();

        for binding in ENV_BINDINGS {
            let Some(expected) = schema.get(binding.key) else {
                mismatches.push(format!(
                    "{}: missing from pkg/config/schema (or tagged no-env)",
                    binding.key
                ));
                continue;
            };
            if binding.env_vars != expected.as_slice() {
                mismatches.push(format!(
                    "{}:\n  ENV_BINDINGS: {:?}\n  schema:      {expected:?}",
                    binding.key, binding.env_vars
                ));
            }
        }

        assert!(
            mismatches.is_empty(),
            "ENV_BINDINGS drifted from pkg/config/schema/yaml:\n{}",
            mismatches.join("\n")
        );
    }

    /// Effective `env_vars` for every setting in the core and system-probe schemas.
    ///
    /// Mirrors `tasks.schema.merge_schema` (`$ref` inlining) and the env fallback in
    /// `tasks.schema.template`: a setting with no `env_vars` and without a `no-env`
    /// tag binds `DD_<KEY_WITH_UNDERSCORES>`.
    fn schema_env_vars() -> HashMap<String, Vec<String>> {
        let dir = schema_yaml_dir();
        let mut out = HashMap::new();
        for top in ["core_schema.yaml", "system-probe_schema.yaml"] {
            let root = load_schema_yaml(&dir.join(top));
            collect_setting_env_vars(&root, "", &mut out);
        }
        out
    }

    fn schema_yaml_dir() -> PathBuf {
        #[cfg(bazel)]
        {
            let marker = std::env::var("DD_CONFIG_SCHEMA_CORE_YAML")
                .expect("DD_CONFIG_SCHEMA_CORE_YAML must be set by the Bazel test target");
            let runfiles = runfiles::Runfiles::create()
                .expect("schema YAML is a runfile, so this test needs `bazel test`");
            let path = runfiles
                .rlocation_from(&marker, option_env!("REPOSITORY_NAME").unwrap_or(""))
                .unwrap_or_else(|| panic!("no runfile for {marker}"));
            return path
                .parent()
                .unwrap_or_else(|| panic!("{} has no parent", path.display()))
                .to_path_buf();
        }
        #[cfg(not(bazel))]
        {
            // pkg/procmgr/rust → pkg/config/schema/yaml
            PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../config/schema/yaml")
        }
    }

    fn load_schema_yaml(path: &Path) -> Value {
        let text = std::fs::read_to_string(path)
            .unwrap_or_else(|err| panic!("read {}: {err}", path.display()));
        let value: Value = serde_yaml::from_str(&text)
            .unwrap_or_else(|err| panic!("parse {}: {err}", path.display()));
        resolve_refs(value, path.parent().expect("schema file parent"))
    }

    /// Inline lone relative `$ref` nodes the way `tasks.schema.merge_schema` does.
    fn resolve_refs(value: Value, base_dir: &Path) -> Value {
        match value {
            Value::Mapping(map) if is_lone_ref(&map) => {
                let ref_name = map
                    .get(Value::String("$ref".into()))
                    .and_then(Value::as_str)
                    .expect("$ref value");
                assert!(
                    !ref_name.contains("://"),
                    "absolute $ref not supported: {ref_name}"
                );
                let mut resolved = load_schema_yaml(&base_dir.join(ref_name));
                if let Value::Mapping(ref mut resolved_map) = resolved {
                    resolved_map.remove(Value::String("$schema".into()));
                    resolved_map.remove(Value::String("$id".into()));
                }
                resolved
            }
            Value::Mapping(map) => Value::Mapping(
                map.into_iter()
                    .map(|(key, child)| (key, resolve_refs(child, base_dir)))
                    .collect(),
            ),
            Value::Sequence(items) => Value::Sequence(
                items
                    .into_iter()
                    .map(|item| resolve_refs(item, base_dir))
                    .collect(),
            ),
            other => other,
        }
    }

    fn is_lone_ref(map: &serde_yaml::Mapping) -> bool {
        map.len() == 1 && map.contains_key(Value::String("$ref".into()))
    }

    fn collect_setting_env_vars(
        node: &Value,
        prefix: &str,
        out: &mut HashMap<String, Vec<String>>,
    ) {
        let Some(props) = node.get("properties").and_then(Value::as_mapping) else {
            return;
        };
        for (key, child) in props {
            let Some(name) = key.as_str() else {
                continue;
            };
            let path = if prefix.is_empty() {
                name.to_owned()
            } else {
                format!("{prefix}.{name}")
            };
            if child.get("node_type").and_then(Value::as_str) == Some("setting") {
                if let Some(env_vars) = effective_env_vars(child, &path) {
                    out.insert(path, env_vars);
                }
            } else {
                collect_setting_env_vars(child, &path, out);
            }
        }
    }

    fn effective_env_vars(setting: &Value, path: &str) -> Option<Vec<String>> {
        if setting_has_tag(setting, "no-env") {
            return None;
        }
        match setting.get("env_vars") {
            Some(Value::Sequence(seq)) if !seq.is_empty() => Some(
                seq.iter()
                    .map(|item| {
                        item.as_str()
                            .unwrap_or_else(|| panic!("{path}: env_vars entry must be a string"))
                            .to_owned()
                    })
                    .collect(),
            ),
            // Omitted or empty: Agent auto name (`tasks.schema.template`).
            _ => Some(vec![auto_env_var_for_key(path)]),
        }
    }

    fn setting_has_tag(setting: &Value, tag: &str) -> bool {
        setting
            .get("tags")
            .and_then(Value::as_sequence)
            .into_iter()
            .flatten()
            .any(|item| item.as_str() == Some(tag))
    }
}
