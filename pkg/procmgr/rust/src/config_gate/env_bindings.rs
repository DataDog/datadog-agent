// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Config-key → environment-variable bindings for procmgr config gates.
//!
//! Source of truth: `env_vars` on each setting in `pkg/config/schema/yaml/`
//! (merged via `dda inv schema.codegen` into generated Go init settings).
//!
//! Keys listed in [`ENV_BINDINGS`] use **only** the named env vars. All other
//! keys use the agent convention `DD_<KEY_WITH_UNDERSCORES>`.
//!
//! This table covers only keys evaluated by config gates (`GATED_KEY_SPECS` in
//! `config_gate.rs`). Keep it in sync with the schema `env_vars` for those keys.
//!
//! On Windows, config gates resolve `DD_*` from the core Agent SCM `Environment`
//! registry first, then dd-procmgr's process environment, so service-local
//! overrides match agent config resolution.

use super::secrets;

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

pub(super) fn env_vars_for_key(key: &str) -> &'static [&'static str] {
    ENV_BINDINGS
        .iter()
        .find(|binding| binding.key == key)
        .map(|binding| binding.env_vars)
        .unwrap_or(&[])
}

pub(super) fn env_bool_for_config_key(
    key: &str,
    agent_yaml: &str,
) -> anyhow::Result<Option<bool>> {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        return env_bool_from_names(names, agent_yaml);
    }
    let auto = auto_env_var_for_key(key);
    env_bool_from_names(&[&auto], agent_yaml)
}

/// Whether any env var bound to `key` is set to a non-empty value (mirrors Go `IsConfigured` env source).
pub(super) fn env_configured_for_key(key: &str) -> bool {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        return names.iter().any(|name| env_var_nonempty(name));
    }
    env_var_nonempty(&auto_env_var_for_key(key))
}

pub(super) fn env_string_for_config_key(key: &str) -> Option<String> {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        for name in names {
            if let Some(value) = env_var_value(name) {
                return Some(value);
            }
        }
        return None;
    }
    env_var_value(&auto_env_var_for_key(key))
}

/// Named env var with Agent SCM override first, then process env (Windows).
pub(super) fn env_var_value_for_name(name: &str) -> Option<String> {
    env_var_value(name)
}

#[cfg(test)]
pub(super) fn all_bound_env_var_names() -> impl Iterator<Item = &'static str> {
    ENV_BINDINGS
        .iter()
        .flat_map(|binding| binding.env_vars.iter().copied())
}

fn auto_env_var_for_key(key: &str) -> String {
    format!("DD_{}", key.replace('.', "_").to_uppercase())
}

fn env_var_nonempty(name: &str) -> bool {
    env_var_value(name).is_some()
}

fn env_bool_from_names(names: &[&str], agent_yaml: &str) -> anyhow::Result<Option<bool>> {
    for name in names {
        if let Some(value) = env_var_value(name) {
            return bool_from_env_string(&value, agent_yaml);
        }
    }
    Ok(None)
}

fn bool_from_env_string(value: &str, agent_yaml: &str) -> anyhow::Result<Option<bool>> {
    if secrets::is_enc(value) {
        let resolved = secrets::try_resolve_config_string(value, agent_yaml)?;
        return Ok(super::parse_agent_bool_string(&resolved));
    }
    Ok(super::parse_agent_bool_string(value).or(Some(false)))
}

/// Core Agent SCM `Environment` overrides, then dd-procmgr process env.
/// SCM wins when present so config gates match agent service-local resolution.
fn env_var_value(name: &str) -> Option<String> {
    if let Some(value) = agent_scm_env_var(name) {
        return Some(value);
    }
    if let Ok(value) = std::env::var(name)
        && !value.is_empty()
    {
        return Some(value);
    }
    None
}

#[cfg(windows)]
fn agent_scm_env_var(name: &str) -> Option<String> {
    crate::platform::core_agent_scm_env_var(name)
}

#[cfg(not(windows))]
fn agent_scm_env_var(_name: &str) -> Option<String> {
    None
}
