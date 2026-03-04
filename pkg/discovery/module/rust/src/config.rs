// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use anyhow::{Context, Result};
use log::warn;
use phf::phf_set;
use std::env;
use std::fs::File;
use std::io::Read;
use std::path::PathBuf;
use yaml_rust2::{Yaml, YamlLoader};

/// Represents the decision for whether to run sd-agent, fallback, or exit
#[derive(Debug, PartialEq)]
pub enum FallbackDecision {
    RunSdAgent,
    FallbackToSystemProbe,
    ExitCleanly,
}

/// Loads the system-probe YAML config file if it exists
pub fn load_config(config_path: Option<PathBuf>) -> Result<Option<Yaml>> {
    let path = config_path.unwrap_or_else(|| PathBuf::from("/etc/datadog-agent/system-probe.yaml"));
    load_config_from_path(&path)
}

/// Derives the core config path (datadog.yaml) using the same resolution logic as
/// Go's `GlobalParams.DatadogConfFilePath()`:
///
/// 1. If `datadog_config_path` is provided:
///    - If it ends with `.yaml`, use it directly as the file path
///    - Otherwise treat it as a directory and append `datadog.yaml`
/// 2. If `datadog_config_path` is `None`, fall back to `sysprobe_config_path`:
///    - If it ends with `.yaml`, use its parent directory + `datadog.yaml`
///    - Otherwise treat it as a directory and append `datadog.yaml`
/// 3. If both are `None`, use the default `/etc/datadog-agent/datadog.yaml`
pub fn load_core_config(
    datadog_config_path: &Option<PathBuf>,
    sysprobe_config_path: &Option<PathBuf>,
) -> Result<Option<Yaml>> {
    let core_path = resolve_core_config_path(datadog_config_path, sysprobe_config_path);
    load_config_from_path(&core_path)
}

fn resolve_core_config_path(
    datadog_config_path: &Option<PathBuf>,
    sysprobe_config_path: &Option<PathBuf>,
) -> PathBuf {
    let default_path = PathBuf::from("/etc/datadog-agent/datadog.yaml");

    if let Some(dd_path) = datadog_config_path {
        if dd_path.extension().is_some_and(|ext| ext == "yaml") {
            return dd_path.clone();
        }
        return dd_path.join("datadog.yaml");
    }

    if let Some(sp_path) = sysprobe_config_path {
        if sp_path.extension().is_some_and(|ext| ext == "yaml") {
            return sp_path
                .parent()
                .map(|p| p.join("datadog.yaml"))
                .unwrap_or(default_path);
        }
        return sp_path.join("datadog.yaml");
    }

    default_path
}

/// Loads a YAML config file from the given path, returning None if it doesn't exist.
fn load_config_from_path(path: &PathBuf) -> Result<Option<Yaml>> {
    if path.exists() {
        let mut file = File::open(path).context("Failed to open config file")?;
        let mut contents = String::new();
        file.read_to_string(&mut contents)
            .context("Failed to read config file")?;

        let docs = YamlLoader::load_from_str(&contents).context("Failed to parse YAML config")?;
        Ok(docs.into_iter().next())
    } else {
        warn!("Config file not found at {}.", path.display());
        Ok(None)
    }
}

/// Get boolean value from YAML, returning Option<bool> instead of defaulting to false
/// This allows us to distinguish between "false" and "not set"
fn get_yaml_bool_option(doc: &Yaml, key: &str) -> Option<bool> {
    let mut current = doc;
    for part in key.split('.') {
        current = &current[part];
        if current.is_badvalue() {
            return None;
        }
    }
    current.as_bool()
}

/// Get string value from YAML, returning Option<String> instead of defaulting to empty string
/// This allows us to distinguish between an empty string and "not set"
fn get_yaml_string_option(doc: &Yaml, key: &str) -> Option<String> {
    let mut current = doc;
    for part in key.split('.') {
        current = &current[part];
        if current.is_badvalue() {
            return None;
        }
    }
    current.as_str().map(|s| s.to_string())
}

/// YAML key paths from system-probe.yaml that trigger non-discovery modules.
/// These keys are derived from Go's enableModules() implementation.
static NON_DISCOVERY_SYSPROBE_YAML_KEYS: phf::Set<&'static str> = phf_set! {
    "ccm_network_config.enabled",
    "compliance_config.database_benchmarks.enabled",
    "dynamic_instrumentation.enabled",
    "ebpf_check.enabled",
    "gpu_monitoring.enabled",
    "network_config.enabled",
    "noisy_neighbor.enabled",
    "ping.enabled",
    "privileged_logs.enabled",
    "runtime_security_config.enabled",
    "runtime_security_config.fim_enabled",
    "service_monitoring_config.enabled",
    "system_probe_config.enable_oom_kill",
    "system_probe_config.enable_tcp_queue_length",
    "system_probe_config.language_detection.enabled",
    "system_probe_config.process_config.enabled",
    "traceroute.enabled",
    "windows_crash_detection.enabled",
};

/// YAML key paths from datadog.yaml (core config) that trigger non-discovery modules.
/// These keys are derived from Go's enableModules() implementation.
static NON_DISCOVERY_CORE_YAML_KEYS: phf::Set<&'static str> = phf_set! {
    "compliance_config.enabled",
    "compliance_config.run_in_system_probe",
    "software_inventory.enabled",
};

/// Returns the YAML key that requires system-probe, if any.
///
/// Checks sysprobe keys against the system-probe.yaml doc and core keys
/// against the datadog.yaml doc. The sets of keys are derived from Go's
/// enableModules() implementation.
fn find_non_discovery_yaml_key(
    sysprobe_doc: &Option<Yaml>,
    core_doc: &Option<Yaml>,
) -> Option<&'static str> {
    if let Some(doc) = sysprobe_doc.as_ref() {
        if !matches!(doc, Yaml::Hash(_)) {
            return Some("<non-hash YAML>");
        }
        if let Some(key) = NON_DISCOVERY_SYSPROBE_YAML_KEYS
            .iter()
            .find(|key| get_yaml_bool_option(doc, key) == Some(true))
        {
            return Some(key);
        }
    }
    if let Some(doc) = core_doc.as_ref() {
        if !matches!(doc, Yaml::Hash(_)) {
            return Some("<non-hash core YAML>");
        }
        if let Some(key) = NON_DISCOVERY_CORE_YAML_KEYS
            .iter()
            .find(|key| get_yaml_bool_option(doc, key) == Some(true))
        {
            return Some(key);
        }
    }
    None
}

/// These keys are derived from Go's enableModules() implementation.
static NON_DISCOVERY_ENV_VARS: phf::Set<&'static str> = phf_set! {
  "DD_CCM_NETWORK_CONFIG_ENABLED",
  "DD_COMPLIANCE_CONFIG_DATABASE_BENCHMARKS_ENABLED",
  "DD_COMPLIANCE_CONFIG_ENABLED",
  "DD_COMPLIANCE_CONFIG_RUN_IN_SYSTEM_PROBE",
  "DD_DYNAMIC_INSTRUMENTATION_ENABLED",
  "DD_EBPF_CHECK_ENABLED",
  "DD_GPU_MONITORING_ENABLED",
  "DD_NOISY_NEIGHBOR_ENABLED",
  "DD_PING_ENABLED",
  "DD_PRIVILEGED_LOGS_ENABLED",
  "DD_RUNTIME_SECURITY_CONFIG_ENABLED",
  "DD_RUNTIME_SECURITY_CONFIG_FIM_ENABLED",
  "DD_SOFTWARE_INVENTORY_ENABLED",
  "DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL",
  "DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH",
  "DD_SYSTEM_PROBE_CONFIG_LANGUAGE_DETECTION_ENABLED",
  "DD_SYSTEM_PROBE_NETWORK_ENABLED",
  "DD_SYSTEM_PROBE_PROCESS_ENABLED",
  "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED",
  "DD_TRACEROUTE_ENABLED",
  "DD_WINDOWS_CRASH_DETECTION_ENABLED",
};

/// Returns the non-discovery environment variable that is set and not
/// explicitly disabled, if any.
///
/// We check the value of each env var rather than just its presence to avoid
/// unnecessary fallback. This is needed because the Helm chart sets feature
/// env vars even for disabled features (e.g. `DD_SYSTEM_PROBE_NETWORK_ENABLED=false`).
///
/// The logic uses `!= Some(false)` so that non-boolean values still trigger
/// fallback as a safety net — matching the YAML side where a section without
/// explicit `enabled: false` triggers fallback.
///
/// Instead of having a list of environment variables we explicitly don't
/// support, it would be better to make this a list of environment variables we
/// _do_ support and ignore everything else like we do with the YAML config. But
/// in some environments the environment for system-probe contains variables
/// from the core agent and system-probe, all without any distinguishing prefix.
///
/// So, until we have an exhaustive list of all system-probe environment
/// variables we don't support, use the approach.
fn find_non_discovery_env_var() -> Option<String> {
    env::vars().find_map(|(key, _)| {
        (NON_DISCOVERY_ENV_VARS.contains(key.as_str()) && get_env_bool_option(&key) != Some(false))
            .then_some(key)
    })
}

fn get_env_bool_option(env_var: &str) -> Option<bool> {
    match env::var(env_var) {
        Ok(val) => {
            let normalized = val.to_lowercase();
            if normalized == "true" || normalized == "1" {
                Some(true)
            } else if normalized == "false" || normalized == "0" {
                Some(false)
            } else {
                None
            }
        }
        Err(_) => None,
    }
}

fn is_config_enabled(env_var: &str, yaml_option: &str, doc: &Option<Yaml>) -> Option<bool> {
    if let Some(enabled) = get_env_bool_option(env_var) {
        return Some(enabled);
    }

    if let Some(doc) = doc
        && let Some(enabled) = get_yaml_bool_option(doc, yaml_option)
    {
        return Some(enabled);
    }

    None
}

/// Gets the sysprobe socket path from configuration
pub fn get_sysprobe_socket_path(config: &Option<Yaml>) -> String {
    // Check environment variable first
    if let Ok(path) = env::var("DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET") {
        return path;
    }

    // Try using the pre-loaded config
    if let Some(doc) = config
        && let Some(path) = get_yaml_string_option(doc, "system_probe_config.sysprobe_socket")
    {
        return path;
    }

    // Default fallback
    "/opt/datadog-agent/run/sysprobe.sock".to_string()
}

/// Parse a Go log level string into a log::Level
/// Unknown levels silently default to Info
fn parse_log_level(level: &str) -> log::Level {
    match level.to_lowercase().as_str() {
        "trace" => log::Level::Trace,
        "debug" => log::Level::Debug,
        "info" => log::Level::Info,
        "warn" | "warning" => log::Level::Warn,
        "error" | "critical" => log::Level::Error,
        "off" => log::Level::Error, // Rust log crate doesn't have "off", use Error as minimal logging
        _ => log::Level::Info,
    }
}

/// Gets the log level from configuration.
/// Priority: DD_LOG_LEVEL > LOG_LEVEL > YAML config > default Info
pub fn get_log_level(config: &Result<Option<Yaml>>) -> log::Level {
    if let Ok(level) = env::var("DD_LOG_LEVEL") {
        return parse_log_level(&level);
    }

    if let Ok(level) = env::var("LOG_LEVEL") {
        return parse_log_level(&level);
    }

    config
        .as_ref()
        .ok()
        .and_then(|opt| opt.as_ref())
        .and_then(|doc| get_yaml_string_option(doc, "log_level"))
        .map(|level| parse_log_level(&level))
        .unwrap_or(log::Level::Info)
}

/// Determines whether to run sd-agent, fallback to system-probe, or exit cleanly.
///
/// `sysprobe_config` is loaded from system-probe.yaml and `core_config` from
/// datadog.yaml.  Each config's YAML keys are checked against the appropriate
/// document so that core-only keys (like `compliance_config.enabled`) are not
/// silently missed when they appear only in datadog.yaml.
pub fn determine_action(
    sysprobe_config: &Result<Option<Yaml>>,
    core_config: &Result<Option<Yaml>>,
) -> FallbackDecision {
    let Some(sysprobe_doc) = sysprobe_config.as_ref().ok() else {
        warn!("Failed to load system-probe YAML config. Falling back to system-probe.");
        return FallbackDecision::FallbackToSystemProbe;
    };
    let use_sd_agent = is_config_enabled(
        "DD_DISCOVERY_USE_SD_AGENT",
        "discovery.use_sd_agent",
        sysprobe_doc,
    );

    if use_sd_agent.is_none_or(|enabled| !enabled) {
        warn!("Falling back to system-probe: sd-agent killswitch is not enabled");
        return FallbackDecision::FallbackToSystemProbe;
    }
    if let Some(var) = find_non_discovery_env_var() {
        warn!("Falling back to system-probe: env var {var} is set");
        return FallbackDecision::FallbackToSystemProbe;
    }

    // core_config failure is non-fatal: we still check sysprobe keys and env vars.
    let core_doc = core_config.as_ref().ok().cloned().unwrap_or(None);

    if let Some(key) = find_non_discovery_yaml_key(sysprobe_doc, &core_doc) {
        warn!("Falling back to system-probe: YAML key {key} is active");
        return FallbackDecision::FallbackToSystemProbe;
    }

    if let Some(enabled) =
        is_config_enabled("DD_DISCOVERY_ENABLED", "discovery.enabled", sysprobe_doc)
        && !enabled
    {
        return FallbackDecision::ExitCleanly;
    }

    // Only discovery is enabled (or no config at all) - run sd-agent
    FallbackDecision::RunSdAgent
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::panic)]
mod tests {
    use super::*;

    use std::io::Write;
    use tempfile::NamedTempFile;

    // Helper to create temp config file
    fn create_test_config(content: &str) -> NamedTempFile {
        let mut file = NamedTempFile::new().unwrap();
        file.write_all(content.as_bytes()).unwrap();
        file.flush().unwrap();
        file
    }

    fn determine_action_no_config() -> FallbackDecision {
        let config_file = create_test_config("");
        let config = load_config(Some(config_file.path().to_path_buf()));
        determine_action(&config, &Ok(None))
    }

    #[test]
    fn test_discovery_only_no_fallback() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
            ],
            || {
                let decision = determine_action_no_config();
                assert_eq!(decision, FallbackDecision::RunSdAgent);
            },
        );
    }

    #[test]
    fn test_network_tracer_needs_fallback() {
        temp_env::with_var("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("true"), || {
            let decision = determine_action_no_config();
            assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
        });
    }

    #[test]
    fn test_env_overrides_yaml() {
        let yaml = r#"
network_config:
  enabled: false
discovery:
  enabled: true
"#;
        let config_file = create_test_config(yaml);

        // Env var says true, YAML says false
        temp_env::with_var("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("true"), || {
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            assert_eq!(
                decision,
                FallbackDecision::FallbackToSystemProbe,
                "Env should override YAML and trigger fallback"
            );
        });
    }

    #[test]
    fn test_yaml_only() {
        let yaml = r#"
network_config:
  enabled: true
discovery:
  enabled: false
"#;
        let config_file = create_test_config(yaml);
        let config = load_config(Some(config_file.path().to_path_buf()));
        let decision = determine_action(&config, &Ok(None));
        assert_eq!(
            decision,
            FallbackDecision::FallbackToSystemProbe,
            "Network tracer from YAML should trigger fallback"
        );
    }

    #[test]
    fn test_invalid_yaml() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = "invalid: yaml: content:\n  bad indentation";
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let result = determine_action(&config, &Ok(None));
            assert!(
                matches!(result, FallbackDecision::FallbackToSystemProbe),
                "Should fallback on invalid YAML"
            );
        });
    }

    #[test]
    fn test_discovery_and_npm_both_enabled() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("true")),
            ],
            || {
                let decision = determine_action_no_config();
                assert_eq!(
                    decision,
                    FallbackDecision::FallbackToSystemProbe,
                    "Should fallback when any unsupported module is enabled"
                );
            },
        );
    }

    #[test]
    fn test_no_modules_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            // Use an empty config file to avoid picking up system config at /etc/datadog-agent/system-probe.yaml
            let decision = determine_action_no_config();
            assert_eq!(decision, FallbackDecision::RunSdAgent);
        });
    }

    // New tests for determine_action() and helper functions

    #[test]
    fn test_get_discovery_state_from_env_enabled() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("true"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), Some(true));
        });
    }

    #[test]
    fn test_get_discovery_state_from_env_enabled_one() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("1"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), Some(true));
        });
    }

    #[test]
    fn test_get_discovery_state_from_env_disabled() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("false"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), Some(false));
        });
    }

    #[test]
    fn test_get_discovery_state_from_env_disabled_zero() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("0"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), Some(false));
        });
    }

    #[test]
    fn test_get_discovery_state_from_env_not_set() {
        // Ensure DD_DISCOVERY_ENABLED is not set
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), None);
        });
    }

    #[test]
    fn test_get_discovery_state_from_env_invalid() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("maybe"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), None);
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_none() {
        // Clean environment - no DD_* vars
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            assert!(find_non_discovery_env_var().is_none());
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_with_dd_foo() {
        temp_env::with_var("DD_DYNAMIC_INSTRUMENTATION_ENABLED", Some("true"), || {
            assert_eq!(
                find_non_discovery_env_var().as_deref(),
                Some("DD_DYNAMIC_INSTRUMENTATION_ENABLED")
            );
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_discovery_only() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("true"), || {
            // DD_DISCOVERY_ENABLED alone should not count as "other" DD_* vars
            assert!(find_non_discovery_env_var().is_none());
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_false_no_fallback() {
        temp_env::with_var("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("false"), || {
            assert!(
                find_non_discovery_env_var().is_none(),
                "Env var set to 'false' should not trigger fallback"
            );
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_zero_no_fallback() {
        temp_env::with_var("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("0"), || {
            assert!(
                find_non_discovery_env_var().is_none(),
                "Env var set to '0' should not trigger fallback"
            );
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_non_boolean_triggers_fallback() {
        temp_env::with_var("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("maybe"), || {
            assert_eq!(
                find_non_discovery_env_var().as_deref(),
                Some("DD_SYSTEM_PROBE_NETWORK_ENABLED"),
            );
        });
    }

    #[test]
    fn test_find_non_discovery_yaml_key_empty() {
        let yaml_doc: Option<Yaml> = None;
        assert!(find_non_discovery_yaml_key(&yaml_doc, &None).is_none());
    }

    #[test]
    fn test_find_non_discovery_yaml_key_discovery_only() {
        let yaml = r#"
discovery:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(find_non_discovery_yaml_key(&yaml_doc, &None).is_none());
    }

    #[test]
    fn test_find_non_discovery_yaml_key_with_other() {
        let yaml = r#"
discovery:
  enabled: true
network_config:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert_eq!(
            find_non_discovery_yaml_key(&yaml_doc, &None),
            Some("network_config.enabled")
        );
    }

    #[test]
    fn test_find_non_discovery_yaml_key_unknown() {
        let yaml = r#"
unknown_key:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert_eq!(
            find_non_discovery_yaml_key(&yaml_doc, &None),
            None,
            "Unknown sections should not trigger fallback in data-driven approach"
        );
    }

    #[test]
    fn test_find_non_discovery_yaml_key_core_config() {
        // Core config keys should be detected in the core doc, not the sysprobe doc
        let core_yaml = r#"
compliance_config:
  enabled: true
"#;
        let core_file = create_test_config(core_yaml);
        let core_doc = load_config(Some(core_file.path().to_path_buf())).unwrap();
        assert_eq!(
            find_non_discovery_yaml_key(&None, &core_doc),
            Some("compliance_config.enabled"),
            "Core config key should be detected in core doc"
        );
    }

    #[test]
    fn test_find_non_discovery_yaml_key_core_software_inventory() {
        let core_yaml = r#"
software_inventory:
  enabled: true
"#;
        let core_file = create_test_config(core_yaml);
        let core_doc = load_config(Some(core_file.path().to_path_buf())).unwrap();
        assert_eq!(
            find_non_discovery_yaml_key(&None, &core_doc),
            Some("software_inventory.enabled"),
            "software_inventory.enabled should be detected in core doc"
        );
    }

    #[test]
    fn test_find_non_discovery_yaml_key_core_key_in_sysprobe_ignored() {
        // If a core key appears in the sysprobe doc, it should NOT be detected
        // (it's only checked against core config)
        let sysprobe_yaml = r#"
compliance_config:
  enabled: true
"#;
        let sp_file = create_test_config(sysprobe_yaml);
        let sp_doc = load_config(Some(sp_file.path().to_path_buf())).unwrap();
        assert_eq!(
            find_non_discovery_yaml_key(&sp_doc, &None),
            None,
            "Core config key in sysprobe doc should not be detected"
        );
    }

    #[test]
    fn test_system_probe_config_with_only_socket() {
        let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  sysprobe_socket: /custom/path.sock
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(
            find_non_discovery_yaml_key(&yaml_doc, &None).is_none(),
            "Should allow system_probe_config with only sysprobe_socket"
        );
    }

    #[test]
    fn test_system_probe_config_with_general_settings() {
        let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  sysprobe_socket: /custom/path.sock
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(
            find_non_discovery_yaml_key(&yaml_doc, &None).is_none(),
            "Should allow system_probe_config with general settings (no tcp_queue_length/oom_kill)"
        );
    }

    #[test]
    fn test_system_probe_config_without_socket() {
        let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(
            find_non_discovery_yaml_key(&yaml_doc, &None).is_none(),
            "Should allow system_probe_config with general settings only"
        );
    }

    #[test]
    fn test_system_probe_config_empty() {
        let yaml = r#"
discovery:
  enabled: true
system_probe_config:
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(
            find_non_discovery_yaml_key(&yaml_doc, &None).is_none(),
            "Should allow empty system_probe_config"
        );
    }

    #[test]
    fn test_determine_action_discovery_only_yaml() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
                "#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            assert_eq!(decision, FallbackDecision::RunSdAgent);
        });
    }

    #[test]
    fn test_determine_action_discovery_only_env() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
            ],
            || {
                let decision = determine_action_no_config();
                assert_eq!(decision, FallbackDecision::RunSdAgent);
            },
        );
    }

    #[test]
    fn test_determine_action_no_config_runs_sd_agent() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let decision = determine_action_no_config();
            assert!(decision == FallbackDecision::RunSdAgent);
        });
    }

    #[test]
    fn test_determine_action_fallback_other_yaml_key() {
        let yaml = r#"
discovery:
  enabled: true
network_config:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let config = load_config(Some(config_file.path().to_path_buf()));
        let decision = determine_action(&config, &Ok(None));
        assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
    }

    #[test]
    fn test_determine_action_fallback_core_yaml_key() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let sp_file = create_test_config("discovery:\n  enabled: true\n");
            let sp_config = load_config(Some(sp_file.path().to_path_buf()));
            let core_file = create_test_config("software_inventory:\n  enabled: true\n");
            let core_config = load_config(Some(core_file.path().to_path_buf()));
            let decision = determine_action(&sp_config, &core_config);
            assert_eq!(
                decision,
                FallbackDecision::FallbackToSystemProbe,
                "Core config key in datadog.yaml should trigger fallback"
            );
        });
    }

    #[test]
    fn test_determine_action_fallback_other_dd_env_var() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", Some("true")),
            ],
            || {
                let decision = determine_action_no_config();
                assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
            },
        );
    }

    #[test]
    fn test_determine_action_exit_cleanly_disabled_yaml() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = r#"
discovery:
  enabled: false
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            // Note: This test may fail if other DD_* vars exist in the environment
            match decision {
                FallbackDecision::ExitCleanly => {}
                FallbackDecision::FallbackToSystemProbe => {
                    // This is OK if there are other DD_* vars in the environment
                    // which would cause fallback instead of clean exit
                }
                _ => panic!("Unexpected decision: {:?}", decision),
            }
        });
    }

    #[test]
    fn test_determine_action_exit_cleanly_disabled_env() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("false")),
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
            ],
            || {
                let decision = determine_action_no_config();
                assert_eq!(decision, FallbackDecision::ExitCleanly);
            },
        );
    }

    #[test]
    fn test_determine_action_disabled_with_other_config_fallback() {
        let yaml = r#"
discovery:
  enabled: false
network_config:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let config = load_config(Some(config_file.path().to_path_buf()));
        let decision = determine_action(&config, &Ok(None));
        assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
    }

    #[test]
    fn test_empty_yaml_file_runs_sd_agent() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = "";
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            match decision {
                FallbackDecision::RunSdAgent => {}
                FallbackDecision::FallbackToSystemProbe => {
                    // This is OK if there are DD_* vars in the environment
                }
                _ => panic!("Expected RunSdAgent, got {:?}", decision),
            }
        });
    }

    #[test]
    fn test_case_insensitive_boolean() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("TRUE"), || {
            assert_eq!(get_env_bool_option("DD_DISCOVERY_ENABLED"), Some(true));
        });
    }

    #[test]
    fn test_get_sysprobe_socket_path_default_when_no_config() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let path = get_sysprobe_socket_path(&None);
            assert_eq!(path, "/opt/datadog-agent/run/sysprobe.sock");
        });
    }

    #[test]
    fn test_get_sysprobe_socket_path_from_env_var() {
        temp_env::with_var(
            "DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET",
            Some("/custom/path.sock"),
            || {
                let path = get_sysprobe_socket_path(&None);
                assert_eq!(path, "/custom/path.sock");
            },
        );
    }

    #[test]
    fn test_get_sysprobe_socket_path_from_yaml() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = r#"
system_probe_config:
  sysprobe_socket: /yaml/path.sock
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf())).unwrap();
            let path = get_sysprobe_socket_path(&config);
            assert_eq!(path, "/yaml/path.sock");
        });
    }

    #[test]
    fn test_get_sysprobe_socket_path_env_overrides_yaml() {
        temp_env::with_var(
            "DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET",
            Some("/env/path.sock"),
            || {
                let yaml = r#"
system_probe_config:
  sysprobe_socket: /yaml/path.sock
"#;
                let config_file = create_test_config(yaml);
                let config = load_config(Some(config_file.path().to_path_buf())).unwrap();
                let path = get_sysprobe_socket_path(&config);
                assert_eq!(
                    path, "/env/path.sock",
                    "Environment variable should override YAML config"
                );
            },
        );
    }

    #[test]
    fn test_get_sysprobe_socket_path_empty_env_string_uses_env() {
        temp_env::with_var("DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET", Some(""), || {
            let yaml = r#"
system_probe_config:
  sysprobe_socket: /yaml/path.sock
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf())).unwrap();
            let path = get_sysprobe_socket_path(&config);
            assert_eq!(
                path, "",
                "Empty string from env var should be used, not fall back to YAML"
            );
        });
    }

    #[test]
    fn test_get_sysprobe_socket_path_yaml_wrong_type() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = r#"
system_probe_config:
  sysprobe_socket: 12345
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf())).unwrap();
            let path = get_sysprobe_socket_path(&config);
            assert_eq!(
                path, "/opt/datadog-agent/run/sysprobe.sock",
                "Should return default when YAML value is not a string"
            );
        });
    }

    #[test]
    fn test_get_sysprobe_socket_path_with_special_chars() {
        temp_env::with_var(
            "DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET",
            Some("/path/to/my socket.sock"),
            || {
                let path = get_sysprobe_socket_path(&None);
                assert_eq!(path, "/path/to/my socket.sock");
            },
        );
    }

    #[test]
    fn test_get_log_level_default_when_no_config() {
        temp_env::with_vars(
            vec![("DD_LOG_LEVEL", None::<&str>), ("LOG_LEVEL", None::<&str>)],
            || {
                let level = get_log_level(&Ok(None));
                assert_eq!(level, log::Level::Info);
            },
        );
    }

    #[test]
    fn test_get_log_level_from_dd_log_level_env() {
        temp_env::with_vars(
            vec![("DD_LOG_LEVEL", Some("debug")), ("LOG_LEVEL", None::<&str>)],
            || {
                let level = get_log_level(&Ok(None));
                assert_eq!(level, log::Level::Debug);
            },
        );
    }

    #[test]
    fn test_get_log_level_from_log_level_env_fallback() {
        temp_env::with_vars(
            vec![("DD_LOG_LEVEL", None::<&str>), ("LOG_LEVEL", Some("trace"))],
            || {
                let level = get_log_level(&Ok(None));
                assert_eq!(level, log::Level::Trace);
            },
        );
    }

    #[test]
    fn test_get_log_level_dd_log_level_overrides_log_level() {
        temp_env::with_vars(
            vec![
                ("DD_LOG_LEVEL", Some("error")),
                ("LOG_LEVEL", Some("trace")),
            ],
            || {
                let level = get_log_level(&Ok(None));
                assert_eq!(
                    level,
                    log::Level::Error,
                    "DD_LOG_LEVEL should take priority over LOG_LEVEL"
                );
            },
        );
    }

    #[test]
    fn test_get_log_level_from_yaml() {
        temp_env::with_vars(
            vec![("DD_LOG_LEVEL", None::<&str>), ("LOG_LEVEL", None::<&str>)],
            || {
                let yaml = r#"
log_level: warn
"#;
                let config_file = create_test_config(yaml);
                let config = load_config(Some(config_file.path().to_path_buf()));
                let level = get_log_level(&config);
                assert_eq!(level, log::Level::Warn);
            },
        );
    }

    #[test]
    fn test_get_log_level_env_overrides_yaml() {
        temp_env::with_var("DD_LOG_LEVEL", Some("error"), || {
            let yaml = r#"
log_level: debug
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let level = get_log_level(&config);
            assert_eq!(
                level,
                log::Level::Error,
                "DD_LOG_LEVEL should override YAML config"
            );
        });
    }

    #[test]
    fn test_get_log_level_empty_env_string_uses_env() {
        temp_env::with_var("DD_LOG_LEVEL", Some(""), || {
            let yaml = r#"
log_level: warn
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let level = get_log_level(&config);
            assert_eq!(
                level,
                log::Level::Info,
                "Empty string from env var should default to Info"
            );
        });
    }

    #[test]
    fn test_get_log_level_yaml_wrong_type() {
        temp_env::with_vars(
            vec![("DD_LOG_LEVEL", None::<&str>), ("LOG_LEVEL", None::<&str>)],
            || {
                let yaml = r#"
log_level: 12345
"#;
                let config_file = create_test_config(yaml);
                let config = load_config(Some(config_file.path().to_path_buf()));
                let level = get_log_level(&config);
                assert_eq!(
                    level,
                    log::Level::Info,
                    "Should return default when YAML value is not a string"
                );
            },
        );
    }

    #[test]
    fn test_get_log_level_case_insensitive() {
        temp_env::with_var("DD_LOG_LEVEL", Some("ERROR"), || {
            let level = get_log_level(&Ok(None));
            assert_eq!(level, log::Level::Error, "Should parse case insensitively");
        });
    }

    #[test]
    fn test_get_log_level_warning_variant() {
        temp_env::with_var("DD_LOG_LEVEL", Some("warning"), || {
            let level = get_log_level(&Ok(None));
            assert_eq!(level, log::Level::Warn, "Should accept 'warning' variant");
        });
    }

    #[test]
    fn test_get_log_level_critical_level() {
        temp_env::with_var("DD_LOG_LEVEL", Some("critical"), || {
            let level = get_log_level(&Ok(None));
            assert_eq!(
                level,
                log::Level::Error,
                "Should map 'critical' to Error level"
            );
        });
    }

    #[test]
    fn test_get_log_level_off_level() {
        temp_env::with_var("DD_LOG_LEVEL", Some("off"), || {
            let level = get_log_level(&Ok(None));
            assert_eq!(level, log::Level::Error, "Should map 'off' to Error level");
        });
    }

    // Killswitch tests
    #[test]
    fn test_killswitch_disabled_forces_fallback() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_USE_SD_AGENT", Some("false")),
                ("DD_DISCOVERY_ENABLED", Some("true")),
            ],
            || {
                let decision = determine_action(&Ok(None), &Ok(None));
                assert_eq!(
                    decision,
                    FallbackDecision::FallbackToSystemProbe,
                    "Should fallback when killswitch is false via env var"
                );
            },
        );
    }

    #[test]
    fn test_killswitch_not_set_defaults_to_fallback() {
        temp_env::with_var("DD_DISCOVERY_ENABLED", Some("true"), || {
            let decision = determine_action(&Ok(None), &Ok(None));
            assert_eq!(
                decision,
                FallbackDecision::FallbackToSystemProbe,
                "Should fallback when killswitch is not set (safe default)"
            );
        });
    }

    #[test]
    fn test_killswitch_enabled_allows_sd_agent() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
                ("DD_DISCOVERY_ENABLED", Some("true")),
            ],
            || {
                let decision = determine_action(&Ok(None), &Ok(None));
                assert_eq!(
                    decision,
                    FallbackDecision::RunSdAgent,
                    "Should run sd-agent when killswitch is true via env var"
                );
            },
        );
    }

    #[test]
    fn test_killswitch_enabled_respects_other_fallback_logic() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_SYSTEM_PROBE_NETWORK_ENABLED", Some("true")), // Non-discovery module
            ],
            || {
                let decision = determine_action(&Ok(None), &Ok(None));
                assert_eq!(
                    decision,
                    FallbackDecision::FallbackToSystemProbe,
                    "Should still fallback for non-discovery modules even when killswitch is true"
                );
            },
        );
    }

    #[test]
    fn test_killswitch_enabled_with_discovery_disabled_exits_cleanly() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_USE_SD_AGENT", Some("true")),
                ("DD_DISCOVERY_ENABLED", Some("false")),
            ],
            || {
                let decision = determine_action(&Ok(None), &Ok(None));
                assert_eq!(
                    decision,
                    FallbackDecision::ExitCleanly,
                    "Should exit cleanly when discovery is disabled, even if killswitch is true"
                );
            },
        );
    }

    // Helm chart scenario tests

    #[test]
    fn test_helm_chart_discovery_only() {
        let yaml = r#"
discovery:
  enabled: true
  use_sd_agent: true
network_config:
  enabled: false
service_monitoring_config:
  enabled: false
runtime_security_config:
  enabled: false
gpu_monitoring:
  enabled: false
traceroute:
  enabled: false
dynamic_instrumentation:
  enabled: false
event_monitoring_config:
  process:
    enabled: false
  network_process:
    enabled: false
system_probe_config:
  sysprobe_socket: /opt/datadog-agent/run/sysprobe.sock
  enabled: true
  enable_tcp_queue_length: false
  enable_oom_kill: false
log_level: info
"#;
        let config_file = create_test_config(yaml);
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            assert_eq!(
                decision,
                FallbackDecision::RunSdAgent,
                "Helm chart with only discovery enabled should run sd-agent"
            );
        });
    }

    #[test]
    fn test_helm_chart_npm_enabled() {
        let yaml = r#"
discovery:
  enabled: true
  use_sd_agent: true
network_config:
  enabled: true
service_monitoring_config:
  enabled: false
runtime_security_config:
  enabled: false
gpu_monitoring:
  enabled: false
traceroute:
  enabled: false
dynamic_instrumentation:
  enabled: false
event_monitoring_config:
  network_process:
    enabled: false
system_probe_config:
  sysprobe_socket: /opt/datadog-agent/run/sysprobe.sock
  enabled: true
  enable_tcp_queue_length: false
  enable_oom_kill: false
log_level: info
"#;
        let config_file = create_test_config(yaml);
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config, &Ok(None));
            assert_eq!(
                decision,
                FallbackDecision::FallbackToSystemProbe,
                "Helm chart with NPM enabled should fallback to system-probe"
            );
        });
    }

    // system_probe_config sub-key tests

    #[test]
    fn test_system_probe_config_tcp_queue_length_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  enable_tcp_queue_length: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::FallbackToSystemProbe
            );
        });
    }

    #[test]
    fn test_system_probe_config_oom_kill_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  enable_oom_kill: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::FallbackToSystemProbe
            );
        });
    }

    #[test]
    fn test_system_probe_config_probes_disabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  enable_tcp_queue_length: false
  enable_oom_kill: false
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::RunSdAgent
            );
        });
    }

    #[test]
    fn test_system_probe_config_general_settings_only() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  enabled: true
  sysprobe_socket: /custom/path.sock
  conntrack:
    enabled: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::RunSdAgent
            );
        });
    }

    #[test]
    fn test_feature_section_without_enabled_key() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
network_config:
  some_other_setting: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::RunSdAgent,
                "Feature section without a known key set to true should not trigger fallback"
            );
        });
    }

    #[test]
    fn test_system_probe_config_process_config_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  process_config:
    enabled: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::FallbackToSystemProbe
            );
        });
    }

    #[test]
    fn test_system_probe_config_process_config_disabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
system_probe_config:
  process_config:
    enabled: false
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config, &Ok(None)),
                FallbackDecision::RunSdAgent
            );
        });
    }

    // resolve_core_config_path tests

    #[test]
    fn test_load_core_config_explicit_directory() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let core_yaml = temp_dir.path().join("datadog.yaml");
        std::fs::write(&core_yaml, "api_key: test123\n").unwrap();

        let dd_path = Some(temp_dir.path().to_path_buf());
        let result = load_core_config(&dd_path, &None).unwrap();
        assert!(
            result.is_some(),
            "Should load datadog.yaml from explicit directory"
        );
    }

    #[test]
    fn test_load_core_config_explicit_yaml_file() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let core_yaml = temp_dir.path().join("datadog.yaml");
        std::fs::write(&core_yaml, "api_key: test123\n").unwrap();

        let dd_path = Some(core_yaml.clone());
        let result = load_core_config(&dd_path, &None).unwrap();
        assert!(
            result.is_some(),
            "Should load datadog.yaml when path ends with .yaml"
        );
    }

    #[test]
    fn test_load_core_config_fallback_to_sysprobe_dir() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let sp_yaml = temp_dir.path().join("system-probe.yaml");
        std::fs::write(&sp_yaml, "discovery:\n  enabled: true\n").unwrap();
        let core_yaml = temp_dir.path().join("datadog.yaml");
        std::fs::write(&core_yaml, "api_key: from_sp_dir\n").unwrap();

        let sp_path = Some(sp_yaml);
        let result = load_core_config(&None, &sp_path).unwrap();
        assert!(
            result.is_some(),
            "Should fallback to sysprobe dir for datadog.yaml"
        );
    }

    #[test]
    fn test_load_core_config_both_none_uses_default() {
        let resolved = resolve_core_config_path(&None, &None);
        assert_eq!(
            resolved,
            PathBuf::from("/etc/datadog-agent/datadog.yaml"),
            "Should use default path when both are None"
        );
    }

    #[test]
    fn test_resolve_core_config_path_explicit_dir() {
        let resolved = resolve_core_config_path(
            &Some(PathBuf::from("/opt/datadog")),
            &Some(PathBuf::from("/etc/system-probe.yaml")),
        );
        assert_eq!(resolved, PathBuf::from("/opt/datadog/datadog.yaml"));
    }

    #[test]
    fn test_resolve_core_config_path_explicit_yaml() {
        let resolved = resolve_core_config_path(
            &Some(PathBuf::from("/opt/datadog/custom.yaml")),
            &Some(PathBuf::from("/etc/system-probe.yaml")),
        );
        assert_eq!(resolved, PathBuf::from("/opt/datadog/custom.yaml"));
    }

    #[test]
    fn test_resolve_core_config_path_fallback_sysprobe_yaml() {
        let resolved =
            resolve_core_config_path(&None, &Some(PathBuf::from("/custom/dir/system-probe.yaml")));
        assert_eq!(resolved, PathBuf::from("/custom/dir/datadog.yaml"));
    }

    #[test]
    fn test_resolve_core_config_path_fallback_sysprobe_dir() {
        let resolved = resolve_core_config_path(&None, &Some(PathBuf::from("/custom/dir")));
        assert_eq!(resolved, PathBuf::from("/custom/dir/datadog.yaml"));
    }
}
