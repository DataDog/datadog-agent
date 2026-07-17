// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Config-key → environment-variable bindings for procmgr config gates.
//!
//! Keys listed in [`ENV_BINDINGS`] use **only** the named env vars (mirrors Go
//! `BindEnvAndSetDefault` when explicit names are passed). All other keys use the
//! agent convention `DD_<KEY_WITH_UNDERSCORES>`.
//!
//! Keep in sync with `pkg/config/setup/process.go` (`procBindEnvAndSetDefault`),
//! `pkg/config/setup/process_settings.go`, `pkg/config/setup/system_probe_settings.go`,
//! and `pkg/config/setup/common_settings.go`.

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

pub(super) fn env_bool_for_config_key(key: &str) -> Option<bool> {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        return env_bool_from_names(names);
    }
    let auto = auto_env_var_for_key(key);
    env_bool_from_names(&[&auto])
}

/// Whether any env var bound to `key` is set (mirrors Go `IsConfigured` env source).
pub(super) fn env_configured_for_key(key: &str) -> bool {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        return names.iter().any(|name| std::env::var(name).is_ok());
    }
    std::env::var(auto_env_var_for_key(key)).is_ok()
}

pub(super) fn env_string_for_config_key(key: &str) -> Option<String> {
    let names = env_vars_for_key(key);
    if !names.is_empty() {
        for name in names {
            if let Ok(value) = std::env::var(name)
                && !value.is_empty()
            {
                return Some(value);
            }
        }
        return None;
    }
    std::env::var(auto_env_var_for_key(key))
        .ok()
        .filter(|value| !value.is_empty())
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

fn env_bool_from_names(names: &[&str]) -> Option<bool> {
    names
        .iter()
        .filter_map(|name| std::env::var(name).ok())
        .find_map(|value| super::parse_bool_string(&value))
}
