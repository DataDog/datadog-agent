// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Merge filtered legacy Windows SCM service `Environment` overrides at spawn time.
//!
//! When procmgr suppresses a legacy SCM service, custom per-service `Environment`
//! registry values are no longer injected by SCM. For known legacy twins (see
//! `cmd/agent/subcommands/run/dependent_services_windows.go`), read
//! `HKLM\SYSTEM\CurrentControlSet\Services\<service>\Environment` on each spawn
//! and merge into the child env block before `processes.d` overrides.

use std::collections::HashMap;
use std::sync::OnceLock;

use super::merge_env_overrides;

/// Core Agent SCM service whose `Environment` registry values drive agent config on Windows.
const CORE_AGENT_SERVICE_NAME: &str = "datadogagent";

static CORE_AGENT_SCM_ENV: OnceLock<HashMap<String, String>> = OnceLock::new();

#[cfg(test)]
static TEST_CORE_AGENT_SCM_ENV: std::sync::Mutex<Option<HashMap<String, String>>> =
    std::sync::Mutex::new(None);

/// Returns a `DD_*` value from the core Agent service SCM `Environment` registry key.
///
/// Used by config gates when dd-procmgr runs under a separate service account/env block
/// and does not inherit datadogagent service-local overrides.
pub(crate) fn core_agent_scm_env_var(name: &str) -> Option<String> {
    #[cfg(test)]
    if let Ok(guard) = TEST_CORE_AGENT_SCM_ENV.lock()
        && let Some(map) = guard.as_ref()
    {
        return scm_env_lookup(map, name);
    }

    let env = CORE_AGENT_SCM_ENV.get_or_init(load_core_agent_scm_environment);
    scm_env_lookup(env, name)
}

fn scm_env_lookup(env: &HashMap<String, String>, name: &str) -> Option<String> {
    env.iter()
        .find(|(key, _)| key.eq_ignore_ascii_case(name))
        .map(|(_, value)| value.clone())
        .filter(|value| !value.is_empty())
}

fn load_core_agent_scm_environment() -> HashMap<String, String> {
    match read_service_environment(CORE_AGENT_SERVICE_NAME) {
        Ok(entries) => parse_scm_environment_entries(&entries)
            .into_iter()
            .collect(),
        Err(e) => {
            log::warn!("failed to read core Agent SCM Environment for config gates: {e:#}");
            HashMap::new()
        }
    }
}

#[cfg(test)]
pub(crate) fn set_test_core_agent_scm_env(env: Option<HashMap<String, String>>) {
    let mut guard = TEST_CORE_AGENT_SCM_ENV
        .lock()
        .expect("test core agent scm env lock");
    *guard = env;
}

/// Procmgr process name → legacy SCM service name (suppressed when procmgr owns the workload).
fn legacy_scm_service_name(process_name: &str) -> Option<&'static str> {
    match process_name {
        "datadog-agent-process" => Some("datadog-process-agent"),
        "datadog-agent-action" => Some("datadog-agent-action"),
        "datadog-agent-ddot" => Some("datadog-otel-agent"),
        _ => None,
    }
}

/// Keys intentionally migrated off SCM/`processes.d` env (registry or yaml resolution).
const LEGACY_SCM_ENV_DENYLIST: &[&str] = &[
    "DD_FLEET_POLICIES_DIR",
    "DD_OTELCOLLECTOR_INSTALLATION_METHOD",
];

/// Build the final child environment: token/process baseline → legacy SCM → processes.d.
pub(crate) fn build_child_env_vars(
    process_name: &str,
    baseline: HashMap<String, String>,
    config_overrides: &[(String, String)],
) -> HashMap<String, String> {
    let mut vars = baseline;
    merge_legacy_scm_env(process_name, &mut vars);
    merge_env_overrides(&mut vars, config_overrides);
    vars
}

/// Merge filtered legacy SCM `Environment` entries into `vars` (before processes.d overrides).
pub(crate) fn merge_legacy_scm_env(process_name: &str, vars: &mut HashMap<String, String>) {
    let Some(service_name) = legacy_scm_service_name(process_name) else {
        return;
    };

    let overrides = match legacy_scm_env_overrides(service_name) {
        Ok(overrides) => overrides,
        Err(e) => {
            log::warn!(
                "[{process_name}] failed to read legacy SCM Environment for {service_name}: {e:#}"
            );
            return;
        }
    };

    if overrides.is_empty() {
        return;
    }

    let names: Vec<&str> = overrides.iter().map(|(k, _)| k.as_str()).collect();
    log::info!(
        "[{process_name}] applying {} legacy SCM environment variable(s) from {service_name}: {}",
        names.len(),
        names.join(", ")
    );
    merge_env_overrides(vars, &overrides);
}

fn legacy_scm_env_overrides(service_name: &str) -> anyhow::Result<Vec<(String, String)>> {
    let entries = read_service_environment(service_name)?;
    Ok(filter_legacy_scm_env(&parse_scm_environment_entries(
        &entries,
    )))
}

fn read_service_environment(service_name: &str) -> anyhow::Result<Vec<String>> {
    use windows_registry::LOCAL_MACHINE;
    use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

    let key = LOCAL_MACHINE
        .options()
        .read()
        .access(KEY_WOW64_64KEY)
        .open(format!(r"SYSTEM\CurrentControlSet\Services\{service_name}"))?;

    match key.get_multi_string("Environment") {
        Ok(entries) => Ok(entries),
        Err(_) => Ok(Vec::new()),
    }
}

fn parse_scm_environment_entries(entries: &[String]) -> Vec<(String, String)> {
    entries
        .iter()
        .filter_map(|entry| {
            let entry = entry.trim();
            if entry.is_empty() {
                return None;
            }
            let (key, value) = entry.split_once('=')?;
            if key.is_empty() {
                return None;
            }
            Some((key.to_string(), value.to_string()))
        })
        .collect()
}

fn filter_legacy_scm_env(entries: &[(String, String)]) -> Vec<(String, String)> {
    entries
        .iter()
        .filter(|(key, _)| !is_denied_legacy_scm_env_key(key))
        .cloned()
        .collect()
}

fn is_denied_legacy_scm_env_key(key: &str) -> bool {
    LEGACY_SCM_ENV_DENYLIST
        .iter()
        .any(|denied| denied.eq_ignore_ascii_case(key))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn parse_scm_environment_entries_skips_empty_and_malformed() {
        let entries = vec![
            "DD_PROXY_HTTP=http://proxy.example.com".to_string(),
            "MALFORMED".to_string(),
            "".to_string(),
            "  DD_LOG_LEVEL=debug  ".to_string(),
        ];
        let parsed = parse_scm_environment_entries(&entries);
        assert_eq!(
            parsed,
            [
                (
                    "DD_PROXY_HTTP".to_string(),
                    "http://proxy.example.com".to_string()
                ),
                ("DD_LOG_LEVEL".to_string(), "debug".to_string()),
            ]
        );
    }

    #[test]
    fn filter_legacy_scm_env_drops_denylisted_keys_case_insensitively() {
        let entries = vec![
            ("DD_PROXY_HTTP".to_string(), "http://x".to_string()),
            ("dd_fleet_policies_dir".to_string(), r"C:\stale".to_string()),
            (
                "DD_OTELCOLLECTOR_INSTALLATION_METHOD".to_string(),
                "bare-metal".to_string(),
            ),
            ("DD_LOG_LEVEL".to_string(), "debug".to_string()),
        ];
        let filtered = filter_legacy_scm_env(&entries);
        assert_eq!(
            filtered,
            [
                ("DD_PROXY_HTTP".to_string(), "http://x".to_string()),
                ("DD_LOG_LEVEL".to_string(), "debug".to_string()),
            ]
        );
    }

    #[test]
    fn build_child_env_vars_applies_legacy_before_processes_d() {
        let mut vars = HashMap::from([("BASE".to_string(), "1".to_string())]);
        merge_env_overrides(
            &mut vars,
            &[("DD_CUSTOM".to_string(), "from-legacy".to_string())],
        );
        merge_env_overrides(
            &mut vars,
            &[("DD_CUSTOM".to_string(), "from-yaml".to_string())],
        );
        assert_eq!(vars.get("DD_CUSTOM").unwrap(), "from-yaml");
        assert_eq!(vars.get("BASE").unwrap(), "1");
    }

    #[test]
    fn core_agent_scm_env_var_uses_case_insensitive_lookup() {
        use std::collections::HashMap;

        set_test_core_agent_scm_env(Some(HashMap::from([(
            "dd_process_config_enabled".to_string(),
            "false".to_string(),
        )])));
        assert_eq!(
            core_agent_scm_env_var("DD_PROCESS_CONFIG_ENABLED"),
            Some("false".to_string())
        );
        set_test_core_agent_scm_env(None);
    }

    #[test]
    fn legacy_scm_service_name_maps_procmgr_managed_processes() {
        assert_eq!(
            legacy_scm_service_name("datadog-agent-process"),
            Some("datadog-process-agent")
        );
        assert_eq!(
            legacy_scm_service_name("datadog-agent-action"),
            Some("datadog-agent-action")
        );
        assert_eq!(
            legacy_scm_service_name("datadog-agent-ddot"),
            Some("datadog-otel-agent")
        );
        assert_eq!(legacy_scm_service_name("datadog-agent-trace"), None);
    }
}
