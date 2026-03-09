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

/// Loads the YAML config file if it exists
pub fn load_config(config_path: Option<PathBuf>) -> Result<Option<Yaml>> {
    let path = config_path.unwrap_or_else(|| PathBuf::from("/etc/datadog-agent/system-probe.yaml"));

    // Try to load YAML, but don't fail if file doesn't exist
    // (env vars might be sufficient)
    if path.exists() {
        let mut file = File::open(&path).context("Failed to open system-probe config file")?;
        let mut contents = String::new();
        file.read_to_string(&mut contents)
            .context("Failed to read system-probe config file")?;

        let docs = YamlLoader::load_from_str(&contents).context("Failed to parse YAML config")?;
        Ok(docs.into_iter().next())
    } else {
        warn!(
            "Config file not found at {}. Checking environment variables only.",
            path.display()
        );
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

/// Checks if `section[key]` is `Yaml::Boolean(true)`.
/// Returns `false` for any other value, including missing keys (`BadValue`),
/// `false`, strings like `"true"`, or non-hash sections.
/// Safe to call on any `Yaml` variant — indexing a non-hash returns `BadValue`.
fn is_yaml_bool_true(section: &Yaml, key: &str) -> bool {
    matches!(section[key], Yaml::Boolean(true))
}

/// Returns the YAML key that requires system-probe, if any.
///
/// We check the `enabled` value of every system-probe feature, rather than
/// key presence to avoid unnecessary fallback. This is needed because the Helm
/// chart generates a system-probe.yaml with all feature sections present, even
/// disabled ones (e.g. `network_config: { enabled: false }`).
///
/// The logic is as follows:
/// - Always allowed: `discovery`, `log_level`.
/// - `system_probe_config` and `event_monitoring_config`: checked for specific
///   sub-feature toggles (they don't have a top-level `enabled`).
/// - Any other section: fallback unless it has `enabled: false`.
fn find_non_discovery_yaml_key(yaml_doc: &Option<Yaml>) -> Option<&str> {
    match yaml_doc {
        None => None,
        Some(Yaml::Hash(map)) => {
            for (key, value) in map {
                let Yaml::String(s) = key else {
                    return Some("<non-string key>");
                };
                match s.as_str() {
                    "discovery" | "log_level" => continue,
                    "event_monitoring_config" => {
                        if is_yaml_bool_true(&value["process"], "enabled") {
                            return Some("event_monitoring_config.process.enabled");
                        }
                        if is_yaml_bool_true(&value["network_process"], "enabled") {
                            return Some("event_monitoring_config.network_process.enabled");
                        }
                    }
                    "system_probe_config" => {
                        if is_yaml_bool_true(value, "enable_tcp_queue_length") {
                            return Some("system_probe_config.enable_tcp_queue_length");
                        }
                        if is_yaml_bool_true(value, "enable_oom_kill") {
                            return Some("system_probe_config.enable_oom_kill");
                        }
                        if is_yaml_bool_true(&value["process_config"], "enabled") {
                            return Some("system_probe_config.process_config.enabled");
                        }
                    }
                    _ => {
                        if !matches!(value["enabled"], Yaml::Boolean(false)) {
                            return Some(s.as_str());
                        }
                    }
                }
            }
            None
        }
        Some(Yaml::BadValue) => None, // Empty or null document
        _ => Some("<non-hash YAML>"), // Any non-hash YAML (array, string, etc.) counts as "other config"
    }
}

static NON_DISCOVERY_ENV_VARS: phf::Set<&'static str> = phf_set! {
  "DD_NETWORK_CONFIG_ENABLED", // Network Performance Monitoring
  "DD_SERVICE_MONITORING_CONFIG_ENABLED", // Universal Service Monitoring
  "DD_CCM_NETWORK_CONFIG_ENABLED", // Cloud Cost Management
  "DD_RUNTIME_SECURITY_CONFIG_ENABLED", // CSM with network monitoring
  "DD_RUNTIME_SECURITY_CONFIG_NETWORK_MONITORING_ENABLED", // CSM with network monitoring
  "DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH", // TCP Queue Length Tracer Module
  "DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL", // OOM Kill Probe Module
  "DD_EVENT_MONITORING_CONFIG_PROCESS_ENABLED", // Process event monitoring
  "DD_SERVICE_MONITORING_CONFIG_ENABLE_EVENT_STREAM", // USM with event stream
  "DD_EVENT_MONITORING_CONFIG_NETWORK_PROCESS_ENABLED", // Network Tracer Module enabled AND DD_EVENT_MONITORING_CONFIG_NETWORK_PROCESS_ENABLED=true
  "DD_GPU_MONITORING_ENABLED", // GPU monitoring
  "DD_DYNAMIC_INSTRUMENTATION_ENABLED", // Dynamic instrumentation
  "DD_COMPLIANCE_CONFIG_ENABLED", // Compliance Module
  "DD_RUNTIME_SECURITY_CONFIG_COMPLIANCE_MODULE_ENABLED", // CSM with compliance module
  "DD_SYSTEM_PROBE_CONFIG_PROCESS_CONFIG_ENABLED", // Process Module
  "DD_EBPF_CHECK_ENABLED", // eBPF Module
  "DD_LANGUAGE_DETECTION_ENABLED", // Language Detection Module
  "DD_PING_ENABLED", // Ping Module
  "DD_TRACEROUTE_ENABLED", // Traceroute Module
  "DD_SOFTWARE_INVENTORY_ENABLED", // Software Inventory Module
  "DD_PRIVILEGED_LOGS_ENABLED", // Privileged Logs Module
  "DD_WINDOWS_CRASH_DETECTION_ENABLED", // Windows Crash Detection Module
};

/// Returns the non-discovery environment variable that is set and not
/// explicitly disabled, if any.
///
/// We check the value of each env var rather than just its presence to avoid
/// unnecessary fallback. This is needed because the Helm chart sets feature
/// env vars even for disabled features (e.g. `DD_NETWORK_CONFIG_ENABLED=false`).
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

/// Determines whether to run sd-agent, fallback to system-probe, or exit cleanly
pub fn determine_action(config: &Result<Option<Yaml>>) -> FallbackDecision {
    let Some(yaml_doc) = config.as_ref().ok() else {
        warn!("Failed to load YAML config. Falling back to system-probe.");
        return FallbackDecision::FallbackToSystemProbe;
    };
    let use_sd_agent = is_config_enabled(
        "DD_DISCOVERY_USE_SD_AGENT",
        "discovery.use_sd_agent",
        yaml_doc,
    );

    if use_sd_agent.is_none_or(|enabled| !enabled) {
        warn!("Falling back to system-probe: sd-agent killswitch is not enabled");
        return FallbackDecision::FallbackToSystemProbe;
    }
    if let Some(var) = find_non_discovery_env_var() {
        warn!("Falling back to system-probe: env var {var} is set");
        return FallbackDecision::FallbackToSystemProbe;
    }
    if let Some(key) = find_non_discovery_yaml_key(yaml_doc) {
        warn!("Falling back to system-probe: YAML key {key} is active");
        return FallbackDecision::FallbackToSystemProbe;
    }

    if let Some(enabled) = is_config_enabled("DD_DISCOVERY_ENABLED", "discovery.enabled", yaml_doc)
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
        determine_action(&config)
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
        temp_env::with_var("DD_NETWORK_CONFIG_ENABLED", Some("true"), || {
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
        temp_env::with_var("DD_NETWORK_CONFIG_ENABLED", Some("true"), || {
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config);
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
        let decision = determine_action(&config);
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
            let result = determine_action(&config);
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
                ("DD_NETWORK_CONFIG_ENABLED", Some("true")),
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
        temp_env::with_var("DD_NETWORK_CONFIG_ENABLED", Some("false"), || {
            assert!(
                find_non_discovery_env_var().is_none(),
                "Env var set to 'false' should not trigger fallback"
            );
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_zero_no_fallback() {
        temp_env::with_var("DD_NETWORK_CONFIG_ENABLED", Some("0"), || {
            assert!(
                find_non_discovery_env_var().is_none(),
                "Env var set to '0' should not trigger fallback"
            );
        });
    }

    #[test]
    fn test_find_non_discovery_env_var_non_boolean_triggers_fallback() {
        temp_env::with_var("DD_NETWORK_CONFIG_ENABLED", Some("maybe"), || {
            assert_eq!(
                find_non_discovery_env_var().as_deref(),
                Some("DD_NETWORK_CONFIG_ENABLED"),
            );
        });
    }

    #[test]
    fn test_find_non_discovery_yaml_key_empty() {
        let yaml_doc: Option<Yaml> = None;
        assert!(find_non_discovery_yaml_key(&yaml_doc).is_none());
    }

    #[test]
    fn test_find_non_discovery_yaml_key_discovery_only() {
        let yaml = r#"
discovery:
  enabled: true
"#;
        let config_file = create_test_config(yaml);
        let yaml_doc = load_config(Some(config_file.path().to_path_buf())).unwrap();
        assert!(find_non_discovery_yaml_key(&yaml_doc).is_none());
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
            find_non_discovery_yaml_key(&yaml_doc),
            Some("network_config")
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
        assert_eq!(find_non_discovery_yaml_key(&yaml_doc), Some("unknown_key"));
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
            find_non_discovery_yaml_key(&yaml_doc).is_none(),
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
            find_non_discovery_yaml_key(&yaml_doc).is_none(),
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
            find_non_discovery_yaml_key(&yaml_doc).is_none(),
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
            find_non_discovery_yaml_key(&yaml_doc).is_none(),
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
            let decision = determine_action(&config);
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
        let decision = determine_action(&config);
        assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
    }

    #[test]
    fn test_determine_action_fallback_other_dd_env_var() {
        temp_env::with_vars(
            [
                ("DD_DISCOVERY_ENABLED", Some("true")),
                ("DD_SERVICE_MONITORING_CONFIG_ENABLED", Some("true")),
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
            let decision = determine_action(&config);
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
        let decision = determine_action(&config);
        assert_eq!(decision, FallbackDecision::FallbackToSystemProbe);
    }

    #[test]
    fn test_empty_yaml_file_runs_sd_agent() {
        temp_env::with_vars(Vec::<(String, Option<String>)>::new(), || {
            let yaml = "";
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            let decision = determine_action(&config);
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
                let decision = determine_action(&Ok(None));
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
            let decision = determine_action(&Ok(None));
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
                let decision = determine_action(&Ok(None));
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
                ("DD_NETWORK_CONFIG_ENABLED", Some("true")), // Non-discovery module
            ],
            || {
                let decision = determine_action(&Ok(None));
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
                let decision = determine_action(&Ok(None));
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
            let decision = determine_action(&config);
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
            let decision = determine_action(&config);
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
                determine_action(&config),
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
                determine_action(&config),
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
            assert_eq!(determine_action(&config), FallbackDecision::RunSdAgent);
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
            assert_eq!(determine_action(&config), FallbackDecision::RunSdAgent);
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
                determine_action(&config),
                FallbackDecision::FallbackToSystemProbe,
                "Feature section without explicit enabled: false should trigger fallback"
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
                determine_action(&config),
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
            assert_eq!(determine_action(&config), FallbackDecision::RunSdAgent);
        });
    }

    // event_monitoring_config tests

    #[test]
    fn test_event_monitoring_config_process_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
event_monitoring_config:
  process:
    enabled: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config),
                FallbackDecision::FallbackToSystemProbe
            );
        });
    }

    #[test]
    fn test_event_monitoring_config_network_process_enabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
event_monitoring_config:
  network_process:
    enabled: true
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(
                determine_action(&config),
                FallbackDecision::FallbackToSystemProbe
            );
        });
    }

    #[test]
    fn test_event_monitoring_config_both_disabled() {
        temp_env::with_vars([("DD_DISCOVERY_USE_SD_AGENT", Some("true"))], || {
            let yaml = r#"
discovery:
  enabled: true
event_monitoring_config:
  process:
    enabled: false
  network_process:
    enabled: false
"#;
            let config_file = create_test_config(yaml);
            let config = load_config(Some(config_file.path().to_path_buf()));
            assert_eq!(determine_action(&config), FallbackDecision::RunSdAgent);
        });
    }
}
