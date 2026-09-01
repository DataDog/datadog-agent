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
//! environment variables, explicit base YAML, infra-mode, then agent default.
//!
//! When deprecated `process_config.enabled` is set in file or env, collection keys
//! follow `loadProcessTransforms` in `pkg/config/setup/process.go`: the deprecated
//! key fills only replacement settings that were not already set (file/env), then
//! normalizes `process_config.enabled` to the resulting process-collection value.
//! The transform runs in `LoadDatadog` before `MergeFleetPolicy`, so fleet-only
//! `process_config.enabled` does not rewrite collection keys, and a transform
//! write (`SourceAgentRuntime`) outranks a later fleet policy.
//!
//! `infrastructure_mode: end_user_device` enables process collection at
//! `SourceInfraMode` (`applyInfrastructureModeOverrides`). That applies only when
//! the key was not set via fleet, env, or file, and was not filled by the legacy
//! transform.
//!
//! Derived `system_probe_config.enabled` (module knobs) is implemented in
//! [`system_probe`] and must stay in sync with `pkg/system-probe/config/config.go`.
//! Env bindings are centralized in [`env_bindings`].

mod env_bindings;
mod secrets;
mod system_probe;
mod yaml_load;

use env_bindings::{env_bool_for_config_key, env_configured_for_key, env_string_for_config_key};

use crate::env::expand_env_vars;
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

impl GatedKeySpec {
    /// Resolution order (most keys): legacy `process_config.enabled` transform
    /// (collection keys not already set in file/env; AgentRuntime, so it outranks
    /// fleet) → fleet policy → env → base YAML → infra-mode (`end_user_device`
    /// enables process collection from pre-fleet env/base only) → agent default.
    ///
    /// `system_probe_config.enabled` is special: returns [`system_probe::derived_enabled`] only,
    /// mirroring post-`load()`/`Adjust` `GetBool` (module-derived runtime value).
    ///
    /// When the transform runs, `process_config.enabled` is rewritten to the
    /// resulting process-collection value. Fleet-only `process_config.enabled`
    /// does not run the transform. Fleet policy outranks env vars
    /// (`SourceFleetPolicies` > `SourceEnvVar`) for keys the transform did not write.
    fn enabled(&self, base_path: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
        if let Some(enabled) = self.legacy_collection_override(base_path, yaml)? {
            return Ok(enabled);
        }
        if let Some(enabled) = self.legacy_enabled_normalized(base_path, yaml)? {
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
        if let Some(enabled) = self.env_override(base_path)? {
            return Ok(enabled);
        }
        if let Some(enabled) = yaml.bool_key_if_exists(base_path, self.key, true)? {
            return Ok(enabled);
        }
        if self.infra_mode_process_collection_override(base_path, yaml)? {
            Ok(true)
        } else {
            Ok(self.default)
        }
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
        // User file/env wins; do not count fleet (transform runs before MergeFleetPolicy).
        if transform_time_user_configured(yaml, base_path, self.key)? {
            return Ok(None);
        }
        let enabled = match self.key {
            "process_config.process_collection.enabled" => mode.process_collection(),
            "process_config.container_collection.enabled" => mode.container_collection(),
            _ => unreachable!(),
        };
        Ok(Some(enabled))
    }

    /// After `loadProcessTransforms`, `process_config.enabled` matches process collection.
    ///
    /// Uses the pre-fleet process-collection value (file/env or the transform fill).
    fn legacy_enabled_normalized(
        &self,
        base_path: &str,
        yaml: &mut YamlCache,
    ) -> anyhow::Result<Option<bool>> {
        if self.key != LEGACY_PROCESS_ENABLED_KEY {
            return Ok(None);
        }
        let Some(mode) = resolve_legacy_process_enabled_mode(base_path, yaml)? else {
            return Ok(None);
        };
        let process_collection =
            transform_time_bool(yaml, base_path, "process_config.process_collection.enabled")?
                .unwrap_or(mode.process_collection());
        Ok(Some(process_collection))
    }

    /// `applyInfrastructureModeOverrides`: `end_user_device` enables process collection
    /// at `SourceInfraMode` (above default, below file/env).
    ///
    /// Runs in `LoadDatadog` before `MergeFleetPolicy` and is not re-run after fleet
    /// merge (unlike ADP overrides), so `infrastructure_mode` is read from env/base
    /// YAML only.
    fn infra_mode_process_collection_override(
        &self,
        base_path: &str,
        yaml: &mut YamlCache,
    ) -> anyhow::Result<bool> {
        if self.key != "process_config.process_collection.enabled" {
            return Ok(false);
        }
        Ok(yaml
            .resolve_string_pre_fleet(base_path, "infrastructure_mode")?
            .is_some_and(|mode| mode == "end_user_device"))
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
        yaml.bool_key_if_exists(&path, self.key, false)
    }

    fn env_override(&self, base_path: &str) -> anyhow::Result<Option<bool>> {
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

fn resolve_config_scalar_string(text: &str, agent_yaml: &str) -> anyhow::Result<Option<String>> {
    if secrets::is_enc(text) {
        let resolved = secrets::try_resolve_config_string(text, agent_yaml)?;
        return Ok(Some(resolved));
    }
    Ok(Some(text.to_owned()))
}

pub(super) struct YamlCache(HashMap<String, serde_yaml::Value>);

impl YamlCache {
    /// Mirrors system-probe/agent fleet policy loading: env → gated config file → registry/default.
    ///
    /// System-probe gates do not inherit `fleet_policies_dir` from sibling `datadog.yaml`
    /// (matches `applyFleetPolicy` on the system-probe config object).
    fn fleet_policies_dir(&mut self, config_path: &str) -> anyhow::Result<Option<String>> {
        let agent_yaml = agent_datadog_yaml(config_path);
        if let Some(dir) = env_bindings::env_var_value_for_name("DD_FLEET_POLICIES_DIR") {
            return Ok(resolve_fleet_policies_dir(&dir, &agent_yaml));
        }
        if let Some(dir) = self.fleet_policies_dir_in_yaml(config_path)? {
            return Ok(resolve_fleet_policies_dir(&dir, &agent_yaml));
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

    fn fleet_policies_dir_in_yaml(&mut self, config_path: &str) -> anyhow::Result<Option<String>> {
        let Some(value) = self.dotted_key_if_exists(config_path, "fleet_policies_dir")? else {
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

    /// Go `GetBool` parity for module derivation (malformed values → false).
    pub(super) fn resolve_get_bool_with_default(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
        default: bool,
    ) -> anyhow::Result<bool> {
        if let Some(filename) = fleet_policy_file
            && let Some(path) = self.fleet_policy_path(filename, base_path)?
            && self.dotted_key_if_exists(&path, key)?.is_some()
        {
            return self.get_bool_at(&path, key);
        }
        if env_configured_for_key(key) {
            return Ok(
                env_bool_for_config_key(key, &agent_datadog_yaml(base_path))?.unwrap_or(false),
            );
        }
        if self.dotted_key_if_exists(base_path, key)?.is_some() {
            return self.get_bool_at_resolving_secrets(base_path, key);
        }
        Ok(default)
    }

    pub(super) fn resolve_get_bool(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: Option<&str>,
    ) -> anyhow::Result<bool> {
        self.resolve_get_bool_with_default(base_path, key, fleet_policy_file, false)
    }

    fn get_bool_at(&mut self, path: &str, key: &str) -> anyhow::Result<bool> {
        Ok(match self.dotted_key_if_exists(path, key)? {
            Some(value) => {
                try_value_as_bool(value, &agent_datadog_yaml(path), false)?.unwrap_or(false)
            }
            None => false,
        })
    }

    fn get_bool_at_resolving_secrets(&mut self, path: &str, key: &str) -> anyhow::Result<bool> {
        Ok(match self.dotted_key_if_exists(path, key)? {
            Some(value) => {
                try_value_as_bool(value, &agent_datadog_yaml(path), true)?.unwrap_or(false)
            }
            None => false,
        })
    }

    /// Env → base YAML (no fleet). Used where Go override funcs run before MergeFleetPolicy.
    pub(super) fn resolve_string_pre_fleet(
        &mut self,
        base_path: &str,
        key: &str,
    ) -> anyhow::Result<Option<String>> {
        let agent_yaml = agent_datadog_yaml(base_path);
        if let Some(text) = env_string_for_config_key(key) {
            return resolve_config_scalar_string(&text, &agent_yaml);
        }
        match self.dotted_key_if_exists(base_path, key)? {
            Some(value) => {
                let Some(text) = Self::string_value(value)? else {
                    return Ok(None);
                };
                resolve_config_scalar_string(&text, &agent_yaml)
            }
            None => Ok(None),
        }
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
                // Unreadable or invalid YAML: Agent `ReadInConfig` still initializes
                // (`ErrConfigFileNotFound` wrap in `pkg/config/nodetreemodel/read_config_file.go`),
                // then env and schema defaults apply. Cache an empty mapping so later keys
                // on the same path do not retry or fail the gate.
                let root = match std::fs::read_to_string(path) {
                    Ok(contents) => match yaml_load::load_yaml(&contents) {
                        Ok(root) => root,
                        Err(err) => {
                            log::warn!(
                                "condition_config_any: parse {path}: {err:#}; using env and defaults"
                            );
                            serde_yaml::Value::Mapping(serde_yaml::Mapping::new())
                        }
                    },
                    Err(err) => {
                        log::warn!(
                            "condition_config_any: read {path}: {err:#}; using env and defaults"
                        );
                        serde_yaml::Value::Mapping(serde_yaml::Mapping::new())
                    }
                };
                Ok(entry.insert(root))
            }
        }
    }

    fn bool_key(
        &mut self,
        path: &str,
        key: &str,
        resolve_secrets: bool,
    ) -> anyhow::Result<Option<bool>> {
        let Some(value) = self.dotted_key(path, key)? else {
            return Ok(None);
        };
        try_value_as_bool(value, &agent_datadog_yaml(path), resolve_secrets)?
            .ok_or_else(|| anyhow::anyhow!("key {key} is not a bool"))
            .map(Some)
    }

    fn bool_key_if_exists(
        &mut self,
        path: &str,
        key: &str,
        resolve_secrets: bool,
    ) -> anyhow::Result<Option<bool>> {
        if !Path::new(path).is_file() {
            return Ok(None);
        }
        self.bool_key(path, key, resolve_secrets)
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

/// Returns true when `conditions` is empty or any `(path, key)` pair is enabled.
pub fn condition_config_any_met(conditions: &[ConditionConfigFile]) -> bool {
    if conditions.is_empty() {
        return true;
    }

    let mut yaml = YamlCache(HashMap::new());
    conditions.iter().any(|file| {
        let path = expand_env_vars(&file.path);
        file.keys.iter().any(|key| {
            config_key_enabled(&path, key, &mut yaml).unwrap_or_else(|err| {
                log::debug!("condition_config_any: {path} key {key}: {err:#}");
                false
            })
        })
    })
}

fn resolve_legacy_process_enabled_mode(
    base_path: &str,
    yaml: &mut YamlCache,
) -> anyhow::Result<Option<ProcessEnabledMode>> {
    // loadProcessTransforms runs in LoadDatadog before MergeFleetPolicy, so only
    // env and the base YAML file participate.
    if let Some(mode) = legacy_enabled_env_mode() {
        return Ok(Some(mode));
    }
    legacy_enabled_mode_from_file(yaml, base_path)
}

/// File or env presence at transform time (`GetSource` > `SourceInfraMode`, before fleet).
fn transform_time_user_configured(
    yaml: &mut YamlCache,
    base_path: &str,
    key: &str,
) -> anyhow::Result<bool> {
    Ok(env_configured_for_key(key) || yaml.key_in_yaml(base_path, key)?)
}

fn transform_time_bool(
    yaml: &mut YamlCache,
    base_path: &str,
    key: &str,
) -> anyhow::Result<Option<bool>> {
    if env_configured_for_key(key) {
        env_bool_for_config_key(key, &agent_datadog_yaml(base_path))
    } else if yaml.key_in_yaml(base_path, key)? {
        yaml.bool_key_if_exists(base_path, key, true)
    } else {
        Ok(None)
    }
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

fn try_value_as_bool(
    value: &serde_yaml::Value,
    agent_yaml: &str,
    resolve_secrets: bool,
) -> anyhow::Result<Option<bool>> {
    match value {
        // Plain YAML 1.1 bools (`yes`/`on`/…) are coerced to bool at load time in [`yaml_load`].
        serde_yaml::Value::Bool(enabled) => Ok(Some(*enabled)),
        // `cast.ToBoolE`: any non-zero number is true, including yaml.v2 floats such as `1.0`.
        serde_yaml::Value::Number(number) => Ok(Some(number_as_bool(number))),
        // Quoted scalars: optional secret resolution then `strconv.ParseBool`.
        serde_yaml::Value::String(text) => {
            if secrets::is_enc(text) {
                if !resolve_secrets {
                    return Ok(None);
                }
                let resolved = secrets::try_resolve_config_string(text, agent_yaml)?;
                return Ok(parse_agent_bool_string(&resolved));
            }
            Ok(parse_agent_bool_string(text).or(Some(false)))
        }
        _ => Ok(None),
    }
}

#[cfg(test)]
fn value_as_bool(value: &serde_yaml::Value, agent_yaml: &str) -> Option<bool> {
    try_value_as_bool(value, agent_yaml, true).ok().flatten()
}

fn number_as_bool(number: &serde_yaml::Number) -> bool {
    if let Some(n) = number.as_i64() {
        n != 0
    } else if let Some(n) = number.as_u64() {
        n != 0
    } else if let Some(n) = number.as_f64() {
        n != 0.0
    } else {
        false
    }
}

/// Mirrors Go `strconv.ParseBool` for env var bindings.
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
            let path = expand_env_vars(&file.path);
            file.keys.iter().map(move |key| format!("{path}:{key}"))
        })
        .collect::<Vec<_>>()
        .join(", ")
}

#[cfg(test)]
pub(crate) mod test_env {
    use std::sync::Mutex;

    static LOCK: Mutex<()> = Mutex::new(());

    /// Serialize tests that mutate process environment (config gates + secret backend).
    pub(crate) fn with_lock<F: FnOnce()>(test: F) {
        let _guard = LOCK.lock().unwrap_or_else(|err| err.into_inner());
        test();
    }

    /// Clear `DD_SECRET_BACKEND_*` overrides so parallel tests cannot hijack backend resolution.
    pub(crate) fn clear_secret_backend_env_vars() {
        const NAMES: &[&str] = &[
            "DD_SECRET_BACKEND_COMMAND",
            "DD_SECRET_BACKEND_ARGUMENTS",
            "DD_SECRET_BACKEND_TYPE",
            "DD_SECRET_BACKEND_CONFIG",
            "DD_SECRET_BACKEND_TIMEOUT",
            "DD_SECRET_BACKEND_OUTPUT_MAX_SIZE",
            "DD_SECRET_BACKEND_REMOVE_TRAILING_LINE_BREAK",
        ];
        for name in NAMES {
            // SAFETY: callers must hold the test env lock.
            unsafe { std::env::remove_var(name) };
        }
    }

    /// `tempfile` directories are mode `0700`; on Linux CI (root) secret backends run as `dd-agent`.
    #[cfg(unix)]
    pub(crate) fn open_tempdir_for_agent_user(path: &std::path::Path) {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o755))
            .expect("open tempdir for agent user");
    }

    #[cfg(not(unix))]
    pub(crate) fn open_tempdir_for_agent_user(_path: &std::path::Path) {}

    pub(crate) fn tempdir_for_secret_backend() -> tempfile::TempDir {
        let dir = tempfile::tempdir().expect("tempdir");
        open_tempdir_for_agent_user(dir.path());
        dir
    }
}

#[cfg(test)]
mod tests {
    use super::test_env;
    use super::*;
    use std::io::Write;
    use std::path::Path;

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

    fn assert_gate_key(agent_path: &str, key: &str, expected: bool) {
        let conditions = vec![ConditionConfigFile {
            path: agent_path.to_string(),
            keys: vec![key.into()],
        }];
        assert_eq!(
            condition_config_any_met(&conditions),
            expected,
            "{key} expected {expected}"
        );
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
        test_env::with_lock(|| {
            test_env::clear_secret_backend_env_vars();
            test();
        });
    }

    fn clear_gated_env_vars() {
        // SAFETY: callers must hold the test env lock.
        unsafe { std::env::remove_var("DD_FLEET_POLICIES_DIR") };
        test_env::clear_secret_backend_env_vars();
        for env_name in super::env_bindings::all_bound_env_var_names() {
            // SAFETY: callers must hold the test env lock.
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
    fn legacy_enabled_numeric_one_does_not_override_explicit_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: 1\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(
                !condition_config_any_met(&process_agent_conditions(agent)),
                "explicit collection/discovery false must win over process_config.enabled"
            );
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
    fn legacy_enabled_negative_prefixed_hex_enables_container_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  enabled: -0x1\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert!(
                condition_config_any_met(&process_agent_conditions(agent)),
                "process_config.enabled: -0x1 must normalize to containers-only like Go GetString(\"-1\")"
            );
        });
    }

    #[test]
    fn multi_document_yaml_uses_first_document_for_gates() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n  process_discovery:\n    enabled: false\n---\nprocess_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert_gate_key(&agent, "process_config.process_collection.enabled", true);
        });
    }

    /// Mirrors `TestProcConfigEnabledTransformPrecedence` in pkg/config/setup/process_test.go.
    #[test]
    fn explicit_container_collection_wins_over_legacy_enabled() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "false");
            let _container =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert_gate_key(&agent, "process_config.container_collection.enabled", false);
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert_gate_key(&agent, "process_config.enabled", false);
        });
    }

    #[test]
    fn explicit_process_collection_wins_over_legacy_enabled() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "true");
            let _process = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert_gate_key(&agent, "process_config.container_collection.enabled", false);
            assert_gate_key(&agent, "process_config.enabled", false);
        });
    }

    #[test]
    fn explicit_collection_settings_win_over_legacy_disabled() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "disabled");
            let _container =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "true");
            let _process = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert_gate_key(&agent, "process_config.container_collection.enabled", true);
            assert_gate_key(&agent, "process_config.process_collection.enabled", true);
            assert_gate_key(&agent, "process_config.enabled", true);
        });
    }

    #[test]
    fn legacy_enabled_still_fills_unset_collection_key() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _legacy = EnvGuard::set("DD_PROCESS_CONFIG_ENABLED", "false");
            let _process = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert_gate_key(&agent, "process_config.process_collection.enabled", true);
            assert_gate_key(&agent, "process_config.container_collection.enabled", true);
            assert_gate_key(&agent, "process_config.enabled", true);
        });
    }

    #[test]
    fn fleet_cannot_restore_normalized_legacy_enabled() {
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
                "process_config:\n  enabled: true\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert_gate_key(&agent, "process_config.enabled", false);
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn end_user_device_enables_process_collection_when_unset() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert_gate_key(&agent, "process_config.process_collection.enabled", true);
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn explicit_process_collection_wins_over_end_user_device() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_end_user_device_does_not_enable_process_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn end_user_device_env_enables_process_collection_when_unset() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _mode = EnvGuard::set("DD_INFRASTRUCTURE_MODE", "end_user_device");
            let _container =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "# api_key: placeholder\n");
            assert_gate_key(&agent, "process_config.process_collection.enabled", true);
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn legacy_transform_overrides_end_user_device_process_collection() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  enabled: disabled\n  process_discovery:\n    enabled: false\n",
            );
            assert_gate_key(&agent, "process_config.process_collection.enabled", false);
            assert!(!condition_config_any_met(&process_agent_conditions(agent)));
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
                )
                .unwrap(),
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
                )
                .unwrap(),
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
                )
                .unwrap(),
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
    fn fleet_policies_dir_from_system_probe_yaml_enables_gate() {
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
            write_config(dir.path(), "datadog.yaml", "# empty\n");
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                &format!(
                    "fleet_policies_dir: {fleet_dir_str}\nnetwork_config:\n  enabled: false\n"
                ),
            );
            let conditions = vec![ConditionConfigFile {
                path: sysprobe,
                keys: vec!["network_config.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_policies_dir_in_datadog_yaml_ignored_for_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            let other_fleet_dir = dir.path().join("other-fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            std::fs::create_dir(&other_fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: true\n",
            );
            write_config(
                &other_fleet_dir,
                "system-probe.yaml",
                "network_config:\n  enabled: false\n",
            );
            let fleet_dir_str = fleet_dir.to_string_lossy();
            let other_fleet_dir_str = other_fleet_dir.to_string_lossy();
            write_config(
                dir.path(),
                "datadog.yaml",
                &format!("fleet_policies_dir: {other_fleet_dir_str}\n"),
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                &format!(
                    "fleet_policies_dir: {fleet_dir_str}\nnetwork_config:\n  enabled: false\n"
                ),
            );
            let conditions = vec![ConditionConfigFile {
                path: sysprobe,
                keys: vec!["network_config.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    /// Linux/non-Windows: no registry fallback, so datadog-only fleet dir must not apply.
    #[cfg(not(windows))]
    #[test]
    fn fleet_policies_dir_only_in_datadog_yaml_ignored_for_system_probe_gate() {
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
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "network_config:\n  enabled: false\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: sysprobe,
                keys: vec!["network_config.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
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
    fn fleet_legacy_enabled_does_not_transform_collection_keys() {
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
                "process_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let collection_only = vec![ConditionConfigFile {
                path: agent.clone(),
                keys: vec![
                    "process_config.process_collection.enabled".into(),
                    "process_config.container_collection.enabled".into(),
                    "process_config.process_discovery.enabled".into(),
                ],
            }];
            assert!(
                !condition_config_any_met(&collection_only),
                "fleet-only process_config.enabled must not rewrite collection keys"
            );
            assert!(
                condition_config_any_met(&process_agent_conditions(agent)),
                "the deprecated key itself still honors fleet policy"
            );
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
            // Linux schema default would otherwise enable discovery with no YAML file.
            let _discovery = EnvGuard::set("DD_DISCOVERY_ENABLED", "false");

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

    /// Env still drives `loadProcessTransforms`; fleet-only `process_config.enabled`
    /// does not rewrite collection keys (`false` means containers-only).
    #[test]
    fn env_legacy_transform_not_overridden_by_fleet_enabled() {
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
            assert!(!condition_config_any_met(&conditions));
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
            cache.bool_key(&path, key, true).unwrap();
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
    fn invalid_base_yaml_uses_default_enabled_keys() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "{\n");
            assert!(
                condition_config_any_met(&process_agent_conditions(agent)),
                "container_collection/process_discovery defaults should keep the gate open"
            );
        });
    }

    #[test]
    fn invalid_base_yaml_keeps_default_disabled_keys_off() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "{\n");
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn env_override_applies_when_base_yaml_is_invalid() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _collection = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true");
            let _container =
                EnvGuard::set("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", "{\n");
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
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
            value_as_bool(&serde_yaml::Value::String("1".into()), agent_yaml),
            Some(true)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::String("yes".into()), agent_yaml),
            Some(false)
        );
        assert_eq!(
            value_as_bool(&serde_yaml::Value::Bool(true), agent_yaml),
            Some(true)
        );
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
    fn condition_config_any_accepts_yaml_11_bool_spellings() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: yes\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn tagged_str_yaml_yes_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: !!str yes\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn quoted_yaml_yes_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: \"yes\"\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn plain_yaml_float_one_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: 1.0\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn plain_yaml_dot_inf_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: .inf\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn plain_yaml_u64_max_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: 18446744073709551615\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn quoted_yaml_float_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: \"1.0\"\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn plain_yaml_hex_one_enables_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: 0x1\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn quoted_yaml_hex_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_discovery:\n    enabled: \"0x1\"\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_discovery.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn env_yes_does_not_enable_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _discovery = EnvGuard::set("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "yes");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
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
    fn malformed_system_probe_bool_does_not_abort_module_derivation() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "network_config:\n  enabled: []\nservice_monitoring_config:\n  enabled: true\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: sysprobe,
                keys: vec!["system_probe_config.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn fleet_infra_mode_change_retains_eud_agent_module_overrides() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(&fleet_dir, "datadog.yaml", "infrastructure_mode: full\n");
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let met = condition_config_any_met(&process_agent_windows_conditions(agent, sysprobe));
            #[cfg(any(windows, target_os = "macos"))]
            assert!(
                met,
                "pre-fleet EUD infra-mode overrides should survive a fleet infrastructure_mode change"
            );
            #[cfg(not(any(windows, target_os = "macos")))]
            assert!(
                !met,
                "Linux has no EUD software_inventory / notable_events modules"
            );
        });
    }

    #[test]
    fn fleet_can_disable_eud_software_inventory_override() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "infrastructure_mode: full\nsoftware_inventory:\n  enabled: false\n",
            );
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "infrastructure_mode: end_user_device\nprocess_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            let met = condition_config_any_met(&process_agent_windows_conditions(agent, sysprobe));
            #[cfg(windows)]
            assert!(
                !met,
                "fleet software_inventory.enabled=false should beat the retained EUD default"
            );
            #[cfg(target_os = "macos")]
            assert!(
                met,
                "retained EUD notable_events should keep the gate open when fleet disables software_inventory only"
            );
            #[cfg(not(any(windows, target_os = "macos")))]
            assert!(
                !met,
                "Linux has no EUD software_inventory / notable_events modules"
            );
        });
    }

    #[test]
    fn derived_notable_events_matches_go_os_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\nnotable_events:\n  enabled: true\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
            let met = condition_config_any_met(&process_agent_windows_conditions(agent, sysprobe));
            #[cfg(target_os = "macos")]
            assert!(
                met,
                "notable_events.enabled should enable system-probe on macOS"
            );
            #[cfg(not(target_os = "macos"))]
            assert!(
                !met,
                "notable_events.enabled should not enable system-probe off macOS"
            );
        });
    }

    #[test]
    fn derived_logon_duration_matches_go_os_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\nlogon_duration:\n  enabled: true\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
            let met = condition_config_any_met(&process_agent_windows_conditions(agent, sysprobe));
            #[cfg(target_os = "macos")]
            assert!(
                met,
                "logon_duration.enabled in datadog.yaml should enable system-probe on macOS"
            );
            #[cfg(not(target_os = "macos"))]
            assert!(
                !met,
                "logon_duration.enabled should not enable system-probe off macOS"
            );
        });
    }

    #[test]
    fn derived_logon_duration_in_system_probe_yaml_does_not_enable() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\nlogon_duration:\n  enabled: true\n",
            );
            assert!(
                !condition_config_any_met(&process_agent_windows_conditions(agent, sysprobe)),
                "logon_duration.enabled in system-probe.yaml should not enable the gate"
            );
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
                "system_probe_config:\n  enabled: true\ndiscovery:\n  enabled: false\n",
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
                "system_probe_config:\n  enabled: true\ndiscovery:\n  enabled: false\n",
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
                "service_monitoring_config:\n  enabled: true\nnetwork_config:\n  enable_sk_tracer: true\ndiscovery:\n  enabled: false\n",
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
                "discovery:\n  enabled: false\n  service_map:\n    enabled: true\nnetwork_config:\n  enable_ebpfless: true\n",
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
                "discovery:\n  enabled: false\n  service_map:\n    enabled: true\nnetwork_config:\n  enable_sk_tracer: true\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    /// Linux default: empty system-probe.yaml enables discovery → system-probe gate open.
    #[cfg(target_os = "linux")]
    #[test]
    fn derived_discovery_linux_default_enables_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }

    /// Fargate platform_default overrides Linux discovery default.
    #[cfg(target_os = "linux")]
    #[test]
    fn derived_discovery_fargate_default_disables_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _fargate = EnvGuard::set("AWS_EXECUTION_ENV", "AWS_ECS_FARGATE");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    /// Explicit YAML false still disables discovery on Linux.
    #[cfg(target_os = "linux")]
    #[test]
    fn derived_discovery_explicit_false_disables_on_linux() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
            assert!(!condition_config_any_met(
                &process_agent_windows_conditions(agent, sysprobe)
            ));
        });
    }

    /// DD_DISCOVERY_ENABLED=false overrides Linux platform default.
    #[cfg(target_os = "linux")]
    #[test]
    fn derived_discovery_env_disable_on_linux() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _discovery = EnvGuard::set("DD_DISCOVERY_ENABLED", "false");

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(dir.path(), "datadog.yaml", ALL_PROCESS_GATES_OFF);
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
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
            let sysprobe = write_config(
                dir.path(),
                "system-probe.yaml",
                "discovery:\n  enabled: false\n",
            );
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
    fn condition_config_any_expands_dd_conf_dir_in_path() {
        with_env_lock(|| {
            clear_gated_env_vars();

            let dir = tempfile::tempdir().unwrap();
            let conf_dir = dir.path().join("agent-conf");
            std::fs::create_dir_all(&conf_dir).unwrap();
            write_config(
                &conf_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n",
            );
            let _conf = EnvGuard::set("DD_CONF_DIR", conf_dir.to_string_lossy().as_ref());
            let conditions = vec![ConditionConfigFile {
                path: "${DD_CONF_DIR}/datadog.yaml".into(),
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(condition_config_any_met(&conditions));
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

    #[test]
    fn fleet_policies_dir_resolves_secret_backed_env_path() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();

            let dir = test_env::tempdir_for_secret_backend();
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
    fn secret_resolved_value_beats_fleet_policy() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();

            let dir = test_env::tempdir_for_secret_backend();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
            );
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("secret_backend.sh");
                std::fs::write(
                    &path,
                    "#!/bin/sh\nprintf '{\"process_collection_enabled\":{\"value\":\"true\"}}'\n",
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
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"process_collection_enabled\\\":{\\\"value\\\":\\\"true\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!(
                    "secret_backend_command: {}\nprocess_config:\n  enabled: false\n  process_collection:\n    enabled: ENC[process_collection_enabled]\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
                    script.to_string_lossy()
                ),
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(condition_config_any_met(&process_agent_conditions(agent)));
        });
    }

    #[test]
    fn fleet_policy_enc_is_not_resolved() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();

            let dir = test_env::tempdir_for_secret_backend();
            let fleet_dir = dir.path().join("fleet");
            std::fs::create_dir(&fleet_dir).unwrap();
            write_config(
                &fleet_dir,
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: ENC[fleet_collection_enabled]\n  process_discovery:\n    enabled: false\n",
            );
            #[cfg(unix)]
            let script = {
                let path = dir.path().join("secret_backend.sh");
                std::fs::write(
                    &path,
                    "#!/bin/sh\nprintf '{\"fleet_collection_enabled\":{\"value\":\"true\"}}'\n",
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
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"fleet_collection_enabled\\\":{\\\"value\\\":\\\"true\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!(
                    "secret_backend_command: {}\n{}",
                    script.to_string_lossy(),
                    ALL_PROCESS_GATES_OFF
                ),
            );
            let _fleet = EnvGuard::set(
                "DD_FLEET_POLICIES_DIR",
                fleet_dir.to_string_lossy().as_ref(),
            );
            assert!(
                !condition_config_any_met(&process_agent_conditions(agent)),
                "fleet policy ENC handles must stay unresolved like Agent MergeFleetPolicy"
            );
        });
    }

    #[test]
    fn env_bool_unresolved_secret_errors_instead_of_falling_through() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _enc = EnvGuard::set(
                "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
                "ENC[missing_backend]",
            );

            assert!(
                env_bool_for_config_key(
                    "process_config.process_collection.enabled",
                    "/nonexistent/datadog.yaml",
                )
                .is_err()
            );
        });
    }

    #[test]
    fn unresolved_env_secret_blocks_gate_despite_yaml_true() {
        with_env_lock(|| {
            clear_gated_env_vars();
            let _enc = EnvGuard::set(
                "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
                "ENC[missing_backend]",
            );

            let dir = tempfile::tempdir().unwrap();
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                "process_config:\n  process_collection:\n    enabled: true\n  process_discovery:\n    enabled: false\n",
            );
            let conditions = vec![ConditionConfigFile {
                path: agent,
                keys: vec!["process_config.process_collection.enabled".into()],
            }];
            assert!(!condition_config_any_met(&conditions));
        });
    }

    #[test]
    fn env_bool_resolves_secret_backed_gate_values() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
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
                env_bool_for_config_key("process_config.process_collection.enabled", &agent)
                    .unwrap(),
                Some(true)
            );
        });
    }

    #[test]
    fn derived_secret_infrastructure_mode_enables_system_probe_gate() {
        with_env_lock(|| {
            clear_gated_env_vars();
            secrets::clear_caches();
            let dir = test_env::tempdir_for_secret_backend();
            #[cfg(unix)]
            let script = dir.path().join("secret_backend.sh");
            #[cfg(unix)]
            {
                std::fs::write(
                    &script,
                    "#!/bin/sh\nprintf '{\"eudm\":{\"value\":\"end_user_device\"}}'\n",
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
                    "@echo off\r\npowershell -NoProfile -Command \"Write-Output '{\\\"eudm\\\":{\\\"value\\\":\\\"end_user_device\\\"}}'\"\r\n",
                )
                .unwrap();
                path
            };
            let agent = write_config(
                dir.path(),
                "datadog.yaml",
                &format!(
                    "secret_backend_command: {}\nprocess_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\ninfrastructure_mode: ENC[eudm]\n",
                    script.to_string_lossy()
                ),
            );
            let sysprobe = write_config(dir.path(), "system-probe.yaml", "# empty\n");
            assert!(condition_config_any_met(&process_agent_windows_conditions(
                agent, sysprobe
            )));
        });
    }
}
