// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Optional `condition_config_any` gates for processes.d definitions.
//!
//! Mirrors the Windows legacy SCM startup checks in
//! `cmd/agent/subcommands/run/dependent_services_windows.go`: start only when any
//! configured key evaluates to true. Resolution order matches agent config
//! (`pkg/config/model/types.go`): agent-runtime transforms, then fleet policy,
//! environment variables, explicit base YAML, then agent default.
//!
//! When deprecated `process_config.enabled` is set, collection keys follow
//! `loadProcessTransforms` in `pkg/config/setup/process.go` instead of defaults.
//!
//! Derived `system_probe_config.enabled` (module knobs) is implemented in
//! [`system_probe`] and must stay in sync with `pkg/system-probe/config/config.go`.
//! Env bindings are centralized in [`env_bindings`].

mod env_bindings;
mod secrets;
mod system_probe;
mod yaml_load;

use env_bindings::{env_bool_for_config_key, env_configured_for_key, env_string_for_config_key};

use serde::Deserialize;
use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::path::Path;

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
    /// Basename under `fleet_policies_dir` when fleet policy overrides apply.
    fleet_policy_file: Option<&'static str>,
}

/// Single source of truth for gated keys (mirrors `pkg/config/setup/process_settings.go`
/// and `pkg/config/setup/system_probe.go`).
const GATED_KEY_SPECS: &[GatedKeySpec] = &[
    GatedKeySpec {
        key: "process_config.enabled",
        default: false,
        fleet_policy_file: Some("datadog.yaml"),
    },
    GatedKeySpec {
        key: "process_config.process_collection.enabled",
        default: false,
        fleet_policy_file: Some("datadog.yaml"),
    },
    GatedKeySpec {
        key: "process_config.container_collection.enabled",
        default: true,
        fleet_policy_file: Some("datadog.yaml"),
    },
    GatedKeySpec {
        key: "process_config.process_discovery.enabled",
        default: true,
        fleet_policy_file: Some("datadog.yaml"),
    },
    GatedKeySpec {
        key: "network_config.enabled",
        default: false,
        fleet_policy_file: Some("system-probe.yaml"),
    },
    GatedKeySpec {
        key: "system_probe_config.enabled",
        default: false,
        fleet_policy_file: Some("system-probe.yaml"),
    },
];

/// Legacy `process_config.enabled` values after `loadProcessTransforms`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ProcessEnabledMode {
    Disabled,
    ProcessesOnly,
    ContainersOnly,
}

impl ProcessEnabledMode {
    fn process_collection(self) -> bool {
        matches!(self, Self::ProcessesOnly)
    }

    fn container_collection(self) -> bool {
        matches!(self, Self::ContainersOnly)
    }
}

const LEGACY_PROCESS_ENABLED_KEY: &str = "process_config.enabled";
const LEGACY_FLEET_POLICY_FILE: &str = "datadog.yaml";

impl GatedKeySpec {
    /// Resolution order (most keys): legacy `process_config.enabled` transform (collection keys only)
    /// → fleet policy → env → base YAML → agent default.
    ///
    /// `system_probe_config.enabled` is special: returns [`system_probe::derived_enabled`] only,
    /// mirroring post-`load()`/`Adjust` `GetBool` (module-derived runtime value).
    ///
    /// Legacy transforms mirror `loadProcessTransforms`. Fleet policy outranks env vars
    /// (`SourceFleetPolicies` > `SourceEnvVar`).
    fn enabled(&self, base_path: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
        if let Some(enabled) = self.legacy_collection_override(base_path, yaml)? {
            return Ok(enabled);
        }
        if self.key == "system_probe_config.enabled" {
            // Mirrors sysprobeConf.GetBool("system_probe_config.enabled") after load()+Adjust:
            // runtime enabled is module-derived, not the literal YAML/env knob alone.
            return system_probe::derived_enabled(base_path, yaml);
        }
        if let Some(enabled) = self.fleet_policy_value(base_path, yaml)? {
            return Ok(enabled);
        }
        if let Some(enabled) = self.env_override(base_path) {
            return Ok(enabled);
        }
        if let Some(enabled) = yaml.bool_key_if_exists(base_path, self.key)? {
            return Ok(enabled);
        }
        Ok(self.default)
    }

    fn uses_legacy_process_enabled(&self) -> bool {
        matches!(
            self.key,
            "process_config.process_collection.enabled"
                | "process_config.container_collection.enabled"
        )
    }

    fn legacy_collection_override(
        &self,
        base_path: &str,
        yaml: &mut YamlCache,
    ) -> anyhow::Result<Option<bool>> {
        if !self.uses_legacy_process_enabled() {
            return Ok(None);
        }
        let Some(mode) = resolve_legacy_process_enabled_mode(base_path, yaml)? else {
            return Ok(None);
        };
        let enabled = match self.key {
            "process_config.process_collection.enabled" => mode.process_collection(),
            "process_config.container_collection.enabled" => mode.container_collection(),
            _ => unreachable!(),
        };
        Ok(Some(enabled))
    }

    fn fleet_policy_value(
        &self,
        base_path: &str,
        yaml: &mut YamlCache,
    ) -> anyhow::Result<Option<bool>> {
        let Some(filename) = self.fleet_policy_file else {
            return Ok(None);
        };
        let Some(path) = yaml.fleet_policy_path(filename, base_path)? else {
            return Ok(None);
        };
        yaml.bool_key_if_exists(&path, self.key)
    }

    fn env_override(&self, base_path: &str) -> Option<bool> {
        env_bool_for_config_key(self.key, &agent_datadog_yaml(base_path))
    }
}

fn agent_datadog_yaml(config_path: &str) -> String {
    let path = Path::new(config_path);
    if path
        .file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| name.eq_ignore_ascii_case("datadog.yaml"))
    {
        return config_path.to_owned();
    }
    path.parent()
        .map(|dir| dir.join("datadog.yaml"))
        .map(|joined| joined.to_string_lossy().into_owned())
        .unwrap_or_else(|| config_path.to_owned())
}

fn resolve_fleet_policies_dir(raw: &str, agent_yaml: &str) -> Option<String> {
    let resolved = secrets::resolve_config_string(raw, agent_yaml);
    if secrets::is_enc(&resolved) || resolved.trim().is_empty() {
        return None;
    }
    Some(resolved)
}

pub(super) struct YamlCache(HashMap<String, serde_yaml::Value>);

impl YamlCache {
    /// Mirrors `pkg/config/setup/config_windows.go` `FleetConfigOverride`: env → datadog.yaml → registry/default.
    fn fleet_policies_dir(&mut self, config_path: &str) -> anyhow::Result<Option<String>> {
        let agent = agent_datadog_yaml(config_path);
        if let Some(dir) = env_bindings::env_var_value_for_name("DD_FLEET_POLICIES_DIR") {
            return Ok(resolve_fleet_policies_dir(&dir, &agent));
        }
        if let Some(dir) = self.fleet_policies_dir_in_yaml(&agent)? {
            return Ok(resolve_fleet_policies_dir(&dir, &agent));
        }
        #[cfg(windows)]
        {
            Ok(crate::platform::fleet_policies_dir_fallback()
                .map(|path| path.to_string_lossy().into_owned()))
        }
        #[cfg(not(windows))]
        {
            Ok(None)
        }
    }

    fn fleet_policies_dir_in_yaml(&mut self, agent_yaml: &str) -> anyhow::Result<Option<String>> {
        let Some(value) = self.dotted_key_if_exists(agent_yaml, "fleet_policies_dir")? else {
            return Ok(None);
        };
        Self::string_value(value)
    }

    fn fleet_policy_path(
        &mut self,
        filename: &str,
        config_path: &str,
    ) -> anyhow::Result<Option<String>> {
        Ok(self.fleet_policies_dir(config_path)?.map(|dir| {
            Path::new(&dir)
                .join(filename)
                .to_string_lossy()
                .into_owned()
        }))
    }

    /// Fleet policy → env bindings → base YAML → `false`.
    pub(super) fn resolve_bool(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
    ) -> anyhow::Result<bool> {
        if let Some(filename) = fleet_policy_file
            && let Some(path) = self.fleet_policy_path(filename, base_path)?
            && let Some(value) = self.bool_key_if_exists(&path, key)?
        {
            return Ok(value);
        }
        if let Some(enabled) = env_bool_for_config_key(key, &agent_datadog_yaml(base_path)) {
            return Ok(enabled);
        }
        Ok(self.bool_key_if_exists(base_path, key)?.unwrap_or(false))
    }

    /// Like [`Self::resolve_bool`] but uses `default` when the key is unset everywhere.
    pub(super) fn resolve_bool_with_default(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
        default: bool,
    ) -> anyhow::Result<bool> {
        if let Some(filename) = fleet_policy_file
            && let Some(path) = self.fleet_policy_path(filename, base_path)?
            && let Some(value) = self.bool_key_if_exists(&path, key)?
        {
            return Ok(value);
        }
        if let Some(enabled) = env_bool_for_config_key(key, &agent_datadog_yaml(base_path)) {
            return Ok(enabled);
        }
        Ok(self.bool_key_if_exists(base_path, key)?.unwrap_or(default))
    }

    pub(super) fn resolve_string(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
    ) -> anyhow::Result<Option<String>> {
        if let Some(filename) = fleet_policy_file
            && let Some(path) = self.fleet_policy_path(filename, base_path)?
            && let Some(value) = self.dotted_key_if_exists(&path, key)?
        {
            return Self::string_value(value);
        }
        if let Some(text) = env_string_for_config_key(key) {
            return Ok(Some(text));
        }
        match self.dotted_key_if_exists(base_path, key)? {
            Some(value) => Self::string_value(value),
            None => Ok(None),
        }
    }

    /// Whether `key` is present in the base YAML file only (not fleet policy or env).
    pub(super) fn key_in_yaml(&mut self, path: &str, key: &str) -> anyhow::Result<bool> {
        Ok(self.dotted_key_if_exists(path, key)?.is_some())
    }

    /// Whether `key` is explicitly set via fleet policy, env, or base YAML.
    ///
    /// Mirrors Go `IsConfigured` for NPM back-compat (`adjust.go`).
    pub(super) fn is_configured(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
    ) -> anyhow::Result<bool> {
        if env_configured_for_key(key) {
            return Ok(true);
        }
        if let Some(filename) = fleet_policy_file
            && let Some(path) = self.fleet_policy_path(filename, base_path)?
            && self.dotted_key_if_exists(&path, key)?.is_some()
        {
            return Ok(true);
        }
        self.key_in_yaml(base_path, key)
    }

    fn string_value(value: &serde_yaml::Value) -> anyhow::Result<Option<String>> {
        match value {
            serde_yaml::Value::String(text) => Ok(Some(text.clone())),
            serde_yaml::Value::Bool(_) | serde_yaml::Value::Number(_) => Ok(None),
            _ => Ok(None),
        }
    }

    fn load(&mut self, path: &str) -> anyhow::Result<&serde_yaml::Value> {
        match self.0.entry(path.to_owned()) {
            Entry::Occupied(entry) => Ok(entry.into_mut()),
            Entry::Vacant(entry) => {
                let contents = std::fs::read_to_string(path)
                    .map_err(|err| anyhow::anyhow!("read {path}: {err}"))?;
                let root = yaml_load::load_yaml(&contents)
                    .map_err(|err| anyhow::anyhow!("parse {path}: {err}"))?;
                Ok(entry.insert(root))
            }
        }
    }

    fn bool_key(&mut self, path: &str, key: &str) -> anyhow::Result<Option<bool>> {
        let Some(value) = self.dotted_key(path, key)? else {
            return Ok(None);
        };
        value_as_bool(value, &agent_datadog_yaml(path))
            .ok_or_else(|| anyhow::anyhow!("key {key} is not a bool"))
            .map(Some)
    }

    fn bool_key_if_exists(&mut self, path: &str, key: &str) -> anyhow::Result<Option<bool>> {
        if !Path::new(path).is_file() {
            return Ok(None);
        }
        self.bool_key(path, key)
    }

    fn dotted_key<'a>(
        &'a mut self,
        path: &str,
        key: &str,
    ) -> anyhow::Result<Option<&'a serde_yaml::Value>> {
        Ok(lookup_dotted_key(self.load(path)?, key))
    }

    pub(super) fn dotted_key_if_exists<'a>(
        &'a mut self,
        path: &str,
        key: &str,
    ) -> anyhow::Result<Option<&'a serde_yaml::Value>> {
        if !Path::new(path).is_file() {
            return Ok(None);
        }
        self.dotted_key(path, key)
    }

    #[cfg(test)]
    fn loaded_file_count(&self) -> usize {
        self.0.len()
    }
}

/// Drop cached secret handles and backend settings before config-gate re-evaluation.
pub(crate) fn clear_secret_caches() {
    secrets::clear_caches();
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

fn resolve_legacy_process_enabled_mode(
    base_path: &str,
    yaml: &mut YamlCache,
) -> anyhow::Result<Option<ProcessEnabledMode>> {
    if let Some(path) = yaml.fleet_policy_path(LEGACY_FLEET_POLICY_FILE, base_path)?
        && let Some(mode) = legacy_enabled_mode_from_file(yaml, &path)?
    {
        return Ok(Some(mode));
    }
    if let Some(mode) = legacy_enabled_env_mode() {
        return Ok(Some(mode));
    }
    legacy_enabled_mode_from_file(yaml, base_path)
}

fn legacy_enabled_mode_from_file(
    yaml: &mut YamlCache,
    path: &str,
) -> anyhow::Result<Option<ProcessEnabledMode>> {
    let Some(value) = yaml.dotted_key_if_exists(path, LEGACY_PROCESS_ENABLED_KEY)? else {
        return Ok(None);
    };
    Ok(legacy_enabled_mode(value))
}

fn legacy_enabled_env_mode() -> Option<ProcessEnabledMode> {
    env_string_for_config_key(LEGACY_PROCESS_ENABLED_KEY)
        .map(|value| legacy_enabled_mode_from_string(&value))
}

fn legacy_enabled_mode(value: &serde_yaml::Value) -> Option<ProcessEnabledMode> {
    legacy_enabled_scalar_as_string(value).map(|text| legacy_enabled_mode_from_string(&text))
}

/// Mirrors Go `GetString` for legacy `process_config.enabled` scalars (string, bool, number).
fn legacy_enabled_scalar_as_string(value: &serde_yaml::Value) -> Option<String> {
    match value {
        serde_yaml::Value::String(text) => Some(text.clone()),
        serde_yaml::Value::Bool(enabled) => Some(enabled.to_string()),
        serde_yaml::Value::Number(number) => {
            if let Some(value) = number.as_i64() {
                return Some(value.to_string());
            }
            if let Some(value) = number.as_u64() {
                return Some(value.to_string());
            }
            number.as_f64().map(|value| {
                if value.fract() == 0.0 && value.is_finite() {
                    format!("{}", value as i64)
                } else {
                    value.to_string()
                }
            })
        }
        _ => None,
    }
}

fn legacy_enabled_mode_from_string(text: &str) -> ProcessEnabledMode {
    // Mirror loadProcessTransforms: ToLower without trim; exact "disabled" match;
    // ParseBool for the true branch; everything else is containers-only.
    let lower = text.to_ascii_lowercase();
    if lower == "disabled" {
        ProcessEnabledMode::Disabled
    } else if parse_agent_bool_string(&lower).unwrap_or(false) {
        ProcessEnabledMode::ProcessesOnly
    } else {
        ProcessEnabledMode::ContainersOnly
    }
}

fn config_key_enabled(path: &str, key: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
    GATED_KEY_SPECS
        .iter()
        .find(|spec| spec.key == key)
        .ok_or_else(|| anyhow::anyhow!("unknown config key {key}"))?
        .enabled(path, yaml)
}

fn lookup_mapping_case_insensitive<'a>(
    mapping: &'a serde_yaml::Mapping,
    key: &str,
) -> Option<&'a serde_yaml::Value> {
    if let Some(value) = mapping.get(key) {
        return Some(value);
    }
    mapping.iter().find_map(|(k, v)| {
        k.as_str()
            .filter(|segment| segment.eq_ignore_ascii_case(key))
            .map(|_| v)
    })
}

fn lookup_dotted_key<'a>(root: &'a serde_yaml::Value, key: &str) -> Option<&'a serde_yaml::Value> {
    lookup_dotted_key_in_mapping(root, key)
}

/// Whether a YAML node counts as an explicit config value for presence/`IsConfigured`.
///
/// Mirrors Agent `read_config_file.go`: known nil leaves (`setting_name:` with no value)
/// are ignored and do not mark the key configured.
fn yaml_value_is_set(value: &serde_yaml::Value) -> bool {
    !matches!(value, serde_yaml::Value::Null)
}

/// Resolves a dotted config key in raw YAML, mirroring Agent flattened keys.
///
/// The Agent expands keys containing `.` into nested maps in
/// `read_config_file.go`; serde_yaml preserves literal dotted keys. At each
/// mapping, try the full remaining key before descending segment-by-segment.
fn lookup_dotted_key_in_mapping<'a>(
    current: &'a serde_yaml::Value,
    key: &str,
) -> Option<&'a serde_yaml::Value> {
    let mapping = current.as_mapping()?;

    if let Some(value) = lookup_mapping_case_insensitive(mapping, key) {
        return yaml_value_is_set(value).then_some(value);
    }

    let (first, rest) = key.split_once('.')?;
    let next = lookup_mapping_case_insensitive(mapping, first)?;
    if !yaml_value_is_set(next) {
        return None;
    }
    lookup_dotted_key_in_mapping(next, rest)
}

fn value_as_bool(value: &serde_yaml::Value, agent_yaml: &str) -> Option<bool> {
    match value {
        serde_yaml::Value::Bool(enabled) => Some(*enabled),
        serde_yaml::Value::Number(number) => yaml_number_as_bool(number),
        serde_yaml::Value::String(text) => bool_from_config_string(text, agent_yaml),
        _ => None,
    }
}

/// Mirrors Go `cast.ToBoolE` for numeric YAML scalars (integers and floats).
fn yaml_number_as_bool(number: &serde_yaml::Number) -> Option<bool> {
    number.as_f64().map(|n| n != 0.0)
}

fn bool_from_config_string(text: &str, agent_yaml: &str) -> Option<bool> {
    let resolved = secrets::resolve_config_string(text, agent_yaml);
    if let Some(enabled) = parse_agent_bool_string(&resolved) {
        return Some(enabled);
    }
    if secrets::is_enc(text) {
        return None;
    }
    Some(parse_agent_bool_string(text).unwrap_or(false))
}

/// Mirrors Go `GetBool` / `strconv.ParseBool` for env and YAML bool strings.
pub(super) fn parse_agent_bool_string(text: &str) -> Option<bool> {
    match text {
        "1" | "t" | "T" => Some(true),
        "0" | "f" | "F" => Some(false),
        _ if text.eq_ignore_ascii_case("true") => Some(true),
        _ if text.eq_ignore_ascii_case("false") => Some(false),
        _ => None,
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

    /// Agent YAML with every process-agent gate key off (including `container_collection` default).
    const ALL_PROCESS_GATES_OFF: &str = "\
process_config:
  process_collection:
    enabled: false
  container_collection:
    enabled: false
  process_discovery:
    enabled: false
";

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

    fn process_agent_windows_conditions(
        agent_path: String,
        sysprobe_path: String,
    ) -> Vec<ConditionConfigFile> {
        vec![
            ConditionConfigFile {
                path: agent_path,
                keys: vec![
                    "process_config.enabled".into(),
                    "process_config.process_collection.enabled".into(),
                    "process_config.container_collection.enabled".into(),
                    "process_config.process_discovery.enabled".into(),
                ],
            },
            ConditionConfigFile {
                path: sysprobe_path,
                keys: vec![
                    "network_config.enabled".into(),
                    "system_probe_config.enabled".into(),
                ],
            },
        ]
    }

    fn with_env_lock<F: FnOnce()>(test: F) {
        let _lock = ENV_TEST_LOCK.lock().unwrap_or_else(|err| err.into_inner());
        test();
    }

    fn clear_gated_env_vars() {
        // SAFETY: callers must hold ENV_TEST_LOCK.
        unsafe { std::env::remove_var("DD_FLEET_POLICIES_DIR") };
        for env_name in super::env_bindings::all_bound_env_var_names() {
            // SAFETY: callers must hold ENV_TEST_LOCK.
            unsafe { std::env::remove_var(env_name) };
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
    fn lookup_dotted_key_is_case_insensitive() {
        let yaml: serde_yaml::Value =
            serde_yaml::from_str("process_config:\n  Process_Collection:\n    enabled: true\n")
                .unwrap();
        assert_eq!(
            lookup_dotted_key(&yaml, "process_config.process_collection.enabled"),
            Some(&serde_yaml::Value::Bool(true))
        );
    }

    #[test]
    fn lookup_dotted_key_supports_flattened_top_level_yaml() {
        let yaml: serde_yaml::Value = serde_yaml::from_str(
            "process_config.process_collection.enabled: true\nprocess_config.container_collection.enabled: false\nprocess_config.process_discovery.enabled: false\n",
        )
        .unwrap();
        assert_eq!(
            lookup_dotted_key(&yaml, "process_config.process_collection.enabled"),
            Some(&serde_yaml::Value::Bool(true))
        );
    }

    #[test]
    fn lookup_dotted_key_treats_null_leaf_as_absent() {
        let yaml: serde_yaml::Value = serde_yaml::from_str(
            "network_config:\n  enabled:\nsystem_probe_config:\n  enabled: true\n",
        )
        .unwrap();
        assert_eq!(lookup_dotted_key(&yaml, "network_config.enabled"), None);
        assert_eq!(
            lookup_dotted_key(&yaml, "system_probe_config.enabled"),
            Some(&serde_yaml::Value::Bool(true))
        );

        let flat: serde_yaml::Value =
            serde_yaml::from_str("network_config.enabled:\nsystem_probe_config.enabled: true\n")
                .unwrap();
        assert_eq!(lookup_dotted_key(&flat, "network_config.enabled"), None);
    }

    #[test]
    fn lookup_dotted_key_supports_partially_flattened_yaml() {
        let yaml: serde_yaml::Value = serde_yaml::from_str(
            "process_config:\n  process_collection.enabled: true\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
        )
        .unwrap();
        assert_eq!(
            lookup_dotted_key(&yaml, "process_config.process_collection.enabled"),
            Some(&serde_yaml::Value::Bool(true))
        );
    }

    #[test]
    fn flattened_yaml_key_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config.process_collection.enabled: true\nprocess_config.container_collection.enabled: false\nprocess_config.process_discovery.enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn duplicate_yaml_key_last_value_wins_for_config_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\nprocess_config:\n  process_collection:\n    enabled: true\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn mixed_case_yaml_key_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  Process_Collection:\n    enabled: true\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
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
                "process_config:\n  enabled: disabled\n  process_discovery:\n    enabled: false\n",
            );
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_false_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_true_enables_process_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: true\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_numeric_one_enables_process_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: 1\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_numeric_zero_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: 0\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn env_override_can_disable_default_enabled_keys() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _collection =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");

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
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn env_bool_and_configured_ignore_empty_values() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _empty = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "");

            assert_eq!(
                env_bool_for_config_key(
                    "process_config.process_discovery.enabled",
                    "/nonexistent/datadog.yaml",
                ),
                None
            );
            assert!(!env_configured_for_key(
                "process_config.process_discovery.enabled"
            ));
        });
    }

    #[test]
    fn env_bool_falls_through_empty_to_next_bound_var() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _empty = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "");
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_DISCOVERY_ENABLED", "true");

            assert_eq!(
                env_bool_for_config_key(
                    "process_config.process_discovery.enabled",
                    "/nonexistent/datadog.yaml",
                ),
                Some(true)
            );
            assert!(env_configured_for_key(
                "process_config.process_discovery.enabled"
            ));
        });
    }

    #[cfg(windows)]
    struct CoreAgentScmEnvGuard;

    #[cfg(windows)]
    impl Drop for CoreAgentScmEnvGuard {
        fn drop(&mut self) {
            crate::platform::set_test_core_agent_scm_env(None);
        }
    }

    #[cfg(windows)]
    #[test]
    fn env_bool_reads_core_agent_scm_when_process_env_unset() {
        use std::collections::HashMap;

        with_env_lock(|| {
            clear_gated_env_vars();
            crate::platform::set_test_core_agent_scm_env(Some(HashMap::from([
                (
                    "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED".to_string(),
                    "false".to_string(),
                ),
                (
                    "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED".to_string(),
                    "false".to_string(),
                ),
            ])));
            let _scm = CoreAgentScmEnvGuard;

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[cfg(windows)]
    #[test]
    fn env_bool_prefers_core_agent_scm_over_process_env() {
        use std::collections::HashMap;

        with_env_lock(|| {
            clear_gated_env_vars();
            crate::platform::set_test_core_agent_scm_env(Some(HashMap::from([(
                "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED".to_string(),
                "false".to_string(),
            )])));
            let _scm = CoreAgentScmEnvGuard;
            let _process = EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "true");

            assert_eq!(
                env_bool_for_config_key(
                    "process_config.container_collection.enabled",
                    "/nonexistent/datadog.yaml",
                ),
                Some(false)
            );
        });
    }

    #[cfg(windows)]
    #[test]
    fn legacy_enabled_env_reads_core_agent_scm_when_process_env_unset() {
        use std::collections::HashMap;

        with_env_lock(|| {
            clear_gated_env_vars();
            crate::platform::set_test_core_agent_scm_env(Some(HashMap::from([
                (
                    "DD_PROCESS_CONFIG_ENABLED".to_string(),
                    "disabled".to_string(),
                ),
                (
                    "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED".to_string(),
                    "false".to_string(),
                ),
                (
                    "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED".to_string(),
                    "false".to_string(),
                ),
            ])));
            let _scm = CoreAgentScmEnvGuard;

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_env_ignores_empty_values() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _empty = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: disabled\n  process_discovery:\n    enabled: false\n",
            );
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_enabled_env_falls_through_empty_to_next_bound_var() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _empty = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "");
            let _agent = EnvGuard::set("DD_PROCESS_AGENT_ENABLED", "disabled");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[cfg(windows)]
    #[test]
    fn fleet_policies_dir_reads_core_agent_scm_env() {
        use std::collections::HashMap;

        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            crate::platform::set_test_core_agent_scm_env(Some(HashMap::from([(
                "DD_FLEET_POLICIES_DIR".to_string(),
                fleet_dir.to_string_lossy().into_owned(),
            )])));
            let _scm = CoreAgentScmEnvGuard;

            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policy_disables_default_enabled_keys() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policy_enables_when_base_config_is_all_false() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policies_dir_from_agent_yaml_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n",
            );
            let fleet_dir_str = fleet_dir.to_string_lossy();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!(
                    "fleet_policies_dir: {fleet_dir_str}\nprocess_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n"
                ),
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policies_dir_resolves_secret_backed_env_path() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n",
            );
            let fleet_dir_json =
                serde_json::to_string(fleet_dir.to_string_lossy().as_ref()).unwrap();
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("secret_backend.sh");
                std::fs::write(
                    &path,
                    format!(
                        "#!/bin/sh\nprintf '{{\"fleet_policies_dir\":{{\"value\":{fleet_dir_json}}}}}'\n"
                    ),
                )
                .unwrap();
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
                path
            };
            #[cfg(windows)]
            let script = {
                let path = dir.path().join("secret_backend.cmd");
                std::fs::write(
                    &path,
                    format!(
                        "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{{\\\"fleet_policies_dir\\\":{{\\\"value\\\":{fleet_dir_json}}}}}'\"\r\n"
                    ),
                )
                .unwrap();
                path
            };
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!(
                    "secret_backend_command: {}\nprocess_config:\n  enabled: false\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
                    script.to_string_lossy()
                ),
            );
            let _fleet = EnvGuard::set("DD_FLEET_POLICIES_DIR", "ENC[fleet_policies_dir]");

            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policies_dir_from_agent_yaml_when_config_path_is_system_probe() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: true\n",
            );
            let fleet_dir_str = fleet_dir.to_string_lossy();
            write_config(
                dir.path(),
                "datadog.yaml",
                &format!("fleet_policies_dir: {fleet_dir_str}\n"),
            );
            let sysprobe = dir.path().join("system-probe.yaml");
            write_config(
                dir.path(),
                "system-probe.yaml",
                "network_config:\n  enabled: false\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: sysprobe.to_string_lossy().into_owned(),
                keys: vec!["network_config.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_system_probe_policy_enables_when_local_file_missing() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = dir.path().join("system-probe.yaml");
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe.to_string_lossy().into_owned(),
                    keys: vec!["network_config.enabled".into()],
                },
            ];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_system_probe_config_policy_enables_when_local_file_missing() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "system_probe_config:\n  enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = dir.path().join("system-probe.yaml");
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe.to_string_lossy().into_owned(),
                    keys: vec!["system_probe_config.enabled".into()],
                },
            ];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_legacy_enabled_transforms_collection_keys() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  enabled: false\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policy_beats_local_system_probe_config() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "network_config:\n  enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe,
                    keys: vec!["network_config.enabled".into()],
                },
            ];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn legacy_env_false_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_env_whitespace_padded_disabled_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", " disabled ");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_yaml_whitespace_padded_disabled_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: \" disabled \"\n  process_discovery:\n    enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn missing_system_probe_without_fleet_blocks_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = dir.path().join("system-probe.yaml");
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe.to_string_lossy().into_owned(),
                    keys: vec![
                        "network_config.enabled".into(),
                        "system_probe_config.enabled".into(),
                    ],
                },
            ];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn local_system_probe_config_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "network_config:\n  enabled: true\n",
            );
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe,
                    keys: vec!["network_config.enabled".into()],
                },
            ];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn env_override_enables_system_probe_network() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _network = EnvGuard::set("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = dir.path().join("system-probe.yaml");
            let conditions = vec![
                ConditionConfigFile {
                    path: agent,
                    keys: vec![
                        "process_config.process_collection.enabled".into(),
                        "process_config.process_discovery.enabled".into(),
                    ],
                },
                ConditionConfigFile {
                    path: sysprobe.to_string_lossy().into_owned(),
                    keys: vec!["network_config.enabled".into()],
                },
            ];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_policy_beats_env_override() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: true\n",
            );
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");
            let _collection =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_legacy_beats_env_for_process_enabled_transform() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  enabled: true\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "false");
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
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
            cache.bool_key(&path, key).unwrap();
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
    fn env_bool_unresolved_secret_falls_through_instead_of_false() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _enc = EnvGuard::set(
                "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
                "ENC[missing_backend]",
            );

            assert_eq!(
                env_bool_for_config_key(
                    "process_config.process_collection.enabled",
                    "/nonexistent/datadog.yaml",
                ),
                None
            );
        });
    }

    #[test]
    fn parse_agent_bool_string_matches_strconv_parse_bool() {
        for (input, expected) in [
            ("1", Some(true)),
            ("t", Some(true)),
            ("T", Some(true)),
            ("true", Some(true)),
            ("TRUE", Some(true)),
            ("True", Some(true)),
            ("0", Some(false)),
            ("f", Some(false)),
            ("F", Some(false)),
            ("false", Some(false)),
            ("FALSE", Some(false)),
            ("False", Some(false)),
            ("yes", None),
            ("on", None),
            ("disabled", None),
            (" true ", None),
            (" false ", None),
            (" 1 ", None),
        ] {
            assert_eq!(parse_agent_bool_string(input), expected, "input={input:?}");
        }
    }

    #[cfg(windows)]
    #[test]
    fn env_bool_scm_whitespace_padded_does_not_enable_process_gates() {
        use std::collections::HashMap;

        with_env_lock(|| {
            clear_gated_env_vars();
            crate::platform::set_test_core_agent_scm_env(Some(HashMap::from([(
                "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED".to_string(),
                " true ".to_string(),
            )])));
            let _scm = CoreAgentScmEnvGuard;

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn env_whitespace_padded_bool_does_not_enable_process_gates() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", " true ");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn value_as_bool_handles_numeric_scalars() {
        let agent_yaml = "/nonexistent/datadog.yaml";
        assert_eq!(
            value_as_bool(&serde_yaml::Value::Number(1.into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::Number(0.into()), agent_yaml),
            Some(false)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::Number(1.0.into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::Number(0.0.into()), agent_yaml),
            Some(false)
        );
    }

    #[test]
    fn value_as_bool_handles_strings() {
        let agent_yaml = "/nonexistent/datadog.yaml";
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("disabled".into()), agent_yaml),
            Some(false)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("true".into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("TRUE".into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("1".into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("t".into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("0".into()), agent_yaml),
            Some(false)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("yes".into()), agent_yaml),
            Some(false)
        );
    }

    #[test]
    fn env_bool_resolves_secret_backed_gate_values() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let dir = tempfile::tempdir().unwrap();
            #[cfg(unix)]
            let script = dir.path().join("secret_backend.sh");
            #[cfg(unix)]
            {
                std::fs::write(
                    &script,
                    "#!/bin/sh\nprintf '{\"process_enabled\":{\"value\":\"true\"}}'\n",
                )
                .unwrap();
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
            }
            #[cfg(windows)]
            let script = {
                let path = dir.path().join("secret_backend.cmd");
                std::fs::write(
                    &path,
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"process_enabled\\\":{\\\"value\\\":\\\"true\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!("secret_backend_command: {}\n", script.to_string_lossy()),
            );
            let _enc = EnvGuard::set(
                "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
                "ENC[process_enabled]",
            );

            assert_eq!(
                env_bool_for_config_key("process_config.process_collection.enabled", &agent,),
                Some(true)
            );
        });
    }

    #[test]
    fn env_yes_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "yes");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn invalid_bool_value_blocks_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: not-a-bool\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn derived_tcp_queue_length_enables_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enable_tcp_queue_length: true\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_module_beats_explicit_system_probe_disabled() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enabled: false\n  enable_oom_kill: true\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_npm_env_disable_blocks_back_compat_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _network = EnvGuard::set("DD_SYSTEM_PROBE_NETWORK_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enabled: true\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn derived_npm_fleet_disable_blocks_back_compat_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: false\n",
            );
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enabled: true\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn derived_npm_back_compat_with_usm_explicitly_disabled() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enabled: true\nservice_monitoring_config:\n  enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_npm_back_compat_ignores_valueless_network_config_enabled() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "system_probe_config:\n  enabled: true\nnetwork_config:\n  enabled:\nservice_monitoring_config:\n  enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_sk_tracer_disables_usm_for_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "service_monitoring_config:\n  enabled: true\nnetwork_config:\n  enable_sk_tracer: true\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn derived_sk_tracer_co_re_env_disables_sk_tracer_gating() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _co_re = EnvGuard::set("DD_ENABLE_CO_RE", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "service_monitoring_config:\n  enabled: true\nnetwork_config:\n  enable_sk_tracer: true\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_discovery_service_map_disabled_when_ebpfless() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  service_map:\n    enabled: true\nnetwork_config:\n  enable_ebpfless: true\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn derived_discovery_service_map_disabled_when_sk_tracer() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  service_map:\n    enabled: true\nnetwork_config:\n  enable_sk_tracer: true\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn derived_usm_env_enables_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _usm = EnvGuard::set("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "service_monitoring_config:\n  enabled: false\n",
            );
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    #[test]
    fn derived_fleet_beats_env_for_module_toggle() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "service_monitoring_config:\n  enabled: false\n",
            );
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let _usm = EnvGuard::set("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true");
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    #[test]
    fn condition_config_summary_formats_paths() {
        let conditions = vec![
            ConditionConfigFile {
                path: "/etc/datadog-agent/datadog.yaml".into(),
                keys: vec![
                    "process_config.enabled".into(),
                    "process_config.process_collection.enabled".into(),
                ],
            },
            ConditionConfigFile {
                path: "/etc/datadog-agent/system-probe.yaml".into(),
                keys: vec!["network_config.enabled".into()],
            },
        ];
        assert_eq!(
            condition_config_summary(&conditions),
            "/etc/datadog-agent/datadog.yaml:process_config.enabled, /etc/datadog-agent/datadog.yaml:process_config.process_collection.enabled, /etc/datadog-agent/system-probe.yaml:network_config.enabled"
        );
    }
}
