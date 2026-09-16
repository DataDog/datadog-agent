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
//! evaluate, so it must be kept in sync with the schema for those keys.
//!
//! Each name is resolved first against the Agent service environment (the Windows SCM
//! `Environment` value, see [`crate::platform::agent_service_env_var`]) and then against
//! dd-procmgr's own environment, so a service-local override is visible to gates exactly
//! as it is to the Agent. An empty value counts as unset, matching
//! `os.LookupEnv(..); ok && value != ""` in `pkg/config/nodetreemodel/config.go`.

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
pub(super) fn env_var_value(name: &str) -> Option<String> {
    if let Some(value) = agent_service_env_var(name) {
        return Some(value);
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
        return vars
            .iter()
            .find(|(key, _)| key.eq_ignore_ascii_case(name))
            .map(|(_, value)| value.clone())
            .filter(|value| !value.is_empty());
    }

    crate::platform::agent_service_env_var(name)
}

#[cfg(test)]
pub(super) fn all_bound_env_var_names() -> impl Iterator<Item = &'static str> {
    ENV_BINDINGS
        .iter()
        .flat_map(|binding| binding.env_vars.iter().copied())
        // Keys without a dedicated binding, so tests can clear the generated name too.
        .chain(["DD_DISCOVERY_ENABLED"])
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
