// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Optional `condition_config_any` and `condition_config_none` gates for processes.d
//! definitions.
//!
//! Mirrors the Windows legacy SCM startup checks in
//! `cmd/agent/subcommands/run/dependent_services_windows.go`: start only when any
//! configured key evaluates to true. A default install leaves every gate open.
//!
//! `condition_config_none` is the veto, for keys that have to be false rather than true.
//! It cannot be expressed as an any-of term, and it is per entry because the same key can
//! be a reason for one process to run and a reason for another not to.
//!
//! # Resolution order
//!
//! For each gated key, highest priority first (mirrors `pkg/config/model/types.go`):
//!
//! 1. The deprecated `process_config.enabled` transform, which writes at
//!    `SourceAgentRuntime` and so outranks fleet policy. See
//!    [`GatedKeySpec::legacy_transform_value`].
//! 2. Fleet policy (`<fleet_policies_dir>/{datadog,system-probe}.yaml`).
//! 3. Environment variables, resolved through [`env_bindings`].
//! 4. The gated YAML file itself.
//! 5. `infrastructure_mode: end_user_device`, which enables process collection and
//!    software inventory at `SourceInfraMode`.
//! 6. The Agent's schema default.
//!
//! Two keys do not follow that ladder directly. `system_probe_config.enabled` is
//! module-derived at runtime, so it resolves through [`system_probe::derived_enabled`]
//! instead. `process_config.enabled` is normalized by the transform to the resulting
//! process-collection value.
//!
//! # Keeping parity with Go
//!
//! Gated keys and their defaults mirror `pkg/config/setup/process_settings.go` and
//! `pkg/config/setup/system_probe.go`. The transform mirrors `loadProcessTransforms`
//! in `pkg/config/setup/process.go`, which runs inside `LoadDatadog` before
//! `MergeFleetPolicy`: the deprecated key fills only replacement settings that
//! file or env did not already set. `applyInfrastructureModeOverrides` runs at the
//! same point and is not re-run after the fleet merge, so `infrastructure_mode` is
//! read from env and base YAML only.
//!
//! Platform differences are carried in [`HostOs`] rather than `cfg`, so every
//! platform's rules can be tested on one runner.

mod env_bindings;
mod system_probe;

use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::path::{Path, PathBuf};

use serde::Deserialize;
use serde_yaml::Value;

use crate::agent_yaml;
use crate::env::expand_env_vars;
use env_bindings::{env_bool_for_key, env_configured_for_key, env_string_for_key};

#[cfg(any(test, feature = "test-helpers"))]
pub use env_bindings::{gate_env_var_names, set_test_agent_service_env};

/// A YAML file and dotted config keys; any key set to true satisfies the gate.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct ConditionConfigFile {
    pub path: String,
    #[serde(default)]
    pub keys: Vec<String>,
}

/// Host operating system, threaded through gate evaluation because several Agent
/// settings and system-probe modules only exist on some platforms.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum HostOs {
    Linux,
    MacOs,
    Windows,
    Other,
}

impl HostOs {
    pub(crate) const CURRENT: Self = if cfg!(windows) {
        Self::Windows
    } else if cfg!(target_os = "macos") {
        Self::MacOs
    } else if cfg!(target_os = "linux") {
        Self::Linux
    } else {
        Self::Other
    };
}

/// Keys a `condition_config_any` gate may name.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum GatedKey {
    /// Deprecated `process_config.enabled`.
    LegacyProcessEnabled,
    ProcessCollection,
    ContainerCollection,
    ProcessDiscovery,
    NetworkConfig,
    SystemProbeConfig,
    WindowsCrashDetection,
    RuntimeSecurity,
    SoftwareInventory,
    SystemProbeExternal,
}

struct GatedKeySpec {
    kind: GatedKey,
    key: &'static str,
    default: bool,
    /// Basename under `fleet_policies_dir` that can override this key.
    fleet_policy_file: &'static str,
}

const AGENT_POLICY: &str = "datadog.yaml";
const SYSPROBE_POLICY: &str = "system-probe.yaml";

const LEGACY_PROCESS_ENABLED_KEY: &str = "process_config.enabled";
const PROCESS_COLLECTION_KEY: &str = "process_config.process_collection.enabled";
const CONTAINER_COLLECTION_KEY: &str = "process_config.container_collection.enabled";
const PROCESS_DISCOVERY_KEY: &str = "process_config.process_discovery.enabled";
const NETWORK_CONFIG_KEY: &str = "network_config.enabled";
const SYSTEM_PROBE_CONFIG_KEY: &str = "system_probe_config.enabled";
const WINDOWS_CRASH_DETECTION_KEY: &str = "windows_crash_detection.enabled";
const RUNTIME_SECURITY_KEY: &str = "runtime_security_config.enabled";
const SOFTWARE_INVENTORY_KEY: &str = "software_inventory.enabled";
const SYSTEM_PROBE_EXTERNAL_KEY: &str = "system_probe_config.external";

/// Single source of truth for gated keys.
const GATED_KEY_SPECS: &[GatedKeySpec] = &[
    GatedKeySpec {
        kind: GatedKey::LegacyProcessEnabled,
        key: LEGACY_PROCESS_ENABLED_KEY,
        default: false,
        fleet_policy_file: AGENT_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::ProcessCollection,
        key: PROCESS_COLLECTION_KEY,
        default: false,
        fleet_policy_file: AGENT_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::ContainerCollection,
        key: CONTAINER_COLLECTION_KEY,
        default: true,
        fleet_policy_file: AGENT_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::ProcessDiscovery,
        key: PROCESS_DISCOVERY_KEY,
        default: true,
        fleet_policy_file: AGENT_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::NetworkConfig,
        key: NETWORK_CONFIG_KEY,
        default: false,
        fleet_policy_file: SYSPROBE_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::SystemProbeConfig,
        key: SYSTEM_PROBE_CONFIG_KEY,
        default: false,
        fleet_policy_file: SYSPROBE_POLICY,
    },
    // The three keys below are named by the system-probe processes.d entry, which
    // transcribes a five-key Servicedef. On Windows the derived
    // `system_probe_config.enabled` above already subsumes all three, so they never
    // change the result of that gate. They are evaluated rather than reported as unknown
    // keys so the transcription is real: a reviewer can diff the template against the
    // Servicedef line by line, and a future drift in the derived mirror does not silently
    // take a module with it.
    GatedKeySpec {
        kind: GatedKey::WindowsCrashDetection,
        key: WINDOWS_CRASH_DETECTION_KEY,
        default: false,
        fleet_policy_file: SYSPROBE_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::RuntimeSecurity,
        key: RUNTIME_SECURITY_KEY,
        default: false,
        fleet_policy_file: SYSPROBE_POLICY,
    },
    GatedKeySpec {
        kind: GatedKey::SoftwareInventory,
        key: SOFTWARE_INVENTORY_KEY,
        default: false,
        fleet_policy_file: AGENT_POLICY,
    },
    // Only ever named by a `condition_config_none`, since it has to be false for
    // system-probe to run. `derived_enabled` reads it too, which covers the derived term
    // on its own; the veto is what covers the literal terms beside it.
    GatedKeySpec {
        kind: GatedKey::SystemProbeExternal,
        key: SYSTEM_PROBE_EXTERNAL_KEY,
        default: false,
        fleet_policy_file: SYSPROBE_POLICY,
    },
];

/// Every key a `condition_config_any` gate can evaluate.
///
/// A shipped template that names anything else takes the unknown-key branch in
/// [`gated_key_enabled`]: the term resolves false and warns on every evaluation, so the
/// gate reads like a transcription while part of it is dead. Exposed so a template test
/// can pin that without evaluating the gate.
#[cfg(all(test, windows))]
pub(crate) fn gated_key_names() -> Vec<&'static str> {
    GATED_KEY_SPECS.iter().map(|spec| spec.key).collect()
}

/// Returns true when `conditions` is empty or any `(path, key)` pair is enabled.
pub fn condition_config_any_met(conditions: &[ConditionConfigFile]) -> bool {
    evaluate(conditions, HostOs::CURRENT)
}

/// Returns true when `conditions` is empty or every `(path, key)` pair is disabled.
///
/// The veto half of the gate. An any-of cannot express "this key must be false", and
/// such a key cannot be folded in beside keys that only have to be true: one term
/// enabling the entry would defeat it. Scoping the veto to the entry is the point, since
/// the same key may be read by another entry that has to keep running.
pub fn condition_config_none_met(conditions: &[ConditionConfigFile]) -> bool {
    evaluate_none(conditions, HostOs::CURRENT)
}

fn evaluate(conditions: &[ConditionConfigFile], os: HostOs) -> bool {
    if conditions.is_empty() {
        return true;
    }

    let mut yaml = YamlCache::default();
    conditions.iter().any(|file| {
        let path = expand_env_vars(&file.path);
        if file.keys.is_empty() {
            log::warn!("condition_config_any: {path} lists no keys, so it can never be satisfied");
        }
        file.keys
            .iter()
            .any(|key| gated_key_enabled(ANY_LABEL, &path, key, &mut yaml, os))
    })
}

fn evaluate_none(conditions: &[ConditionConfigFile], os: HostOs) -> bool {
    if conditions.is_empty() {
        return true;
    }

    let mut yaml = YamlCache::default();
    !conditions.iter().any(|file| {
        let path = expand_env_vars(&file.path);
        if file.keys.is_empty() {
            log::warn!("condition_config_none: {path} lists no keys, so it vetoes nothing");
        }
        file.keys
            .iter()
            .any(|key| gated_key_enabled(NONE_LABEL, &path, key, &mut yaml, os))
    })
}

const ANY_LABEL: &str = "condition_config_any";
const NONE_LABEL: &str = "condition_config_none";

/// An unknown key resolves false, which is restrictive for an any-of (the term can never
/// open the gate) and permissive for a veto (it can never close it). Neither direction is
/// safe on its own, so the miss is not the remedy: it warns on every evaluation, and the
/// `fleet_*_template` tests assert the shipped templates name nothing outside
/// [`GATED_KEY_SPECS`].
fn gated_key_enabled(label: &str, path: &str, key: &str, yaml: &mut YamlCache, os: HostOs) -> bool {
    match GATED_KEY_SPECS.iter().find(|spec| spec.key == key) {
        Some(spec) => spec.enabled(path, yaml, os),
        None => {
            log::warn!("{label}: unknown config key {key} in {path}");
            false
        }
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

impl GatedKeySpec {
    /// Resolves this key against `base_path`, following the order documented on the module.
    fn enabled(&self, base_path: &str, yaml: &mut YamlCache, os: HostOs) -> bool {
        if let Some(enabled) = self.legacy_transform_value(base_path, yaml) {
            return enabled;
        }
        if self.kind == GatedKey::SystemProbeConfig {
            // Mirrors sysprobeConf.GetBool("system_probe_config.enabled") after load()+Adjust:
            // the runtime value is module-derived, not the literal YAML/env knob alone.
            return system_probe::derived_enabled(base_path, yaml, os);
        }
        if let Some(enabled) = yaml.resolve_bool(base_path, self.key, self.fleet_policy_file) {
            return enabled;
        }
        // `applyInfrastructureModeOverrides` turns both of these on for `end_user_device`
        // when no source set them.
        if matches!(
            self.kind,
            GatedKey::ProcessCollection | GatedKey::SoftwareInventory
        ) && yaml.end_user_device_at_load(base_path)
        {
            return true;
        }
        self.default
    }

    /// `loadProcessTransforms`: the deprecated `process_config.enabled` fills collection
    /// keys that file or env did not set, then normalizes itself to the resulting
    /// process-collection value.
    ///
    /// Returns `None` when the transform does not apply, leaving the normal ladder to
    /// run. Fleet policy is deliberately not an input: the transform runs before
    /// `MergeFleetPolicy`, so a fleet-only `process_config.enabled` cannot rewrite
    /// collection keys, and a value the transform did write outranks fleet policy.
    fn legacy_transform_value(&self, base_path: &str, yaml: &mut YamlCache) -> Option<bool> {
        let filled = match self.kind {
            GatedKey::LegacyProcessEnabled | GatedKey::ProcessCollection => {
                GatedKey::ProcessCollection
            }
            GatedKey::ContainerCollection => GatedKey::ContainerCollection,
            _ => return None,
        };
        let mode = legacy_process_enabled_mode(base_path, yaml)?;

        if self.kind == GatedKey::LegacyProcessEnabled {
            let user_value = transform_time_bool(yaml, base_path, PROCESS_COLLECTION_KEY);
            return Some(user_value.unwrap_or(mode.enables(filled)));
        }
        if transform_time_configured(yaml, base_path, self.key) {
            return None;
        }
        Some(mode.enables(filled))
    }
}

/// Legacy `process_config.enabled` values, as interpreted by `loadProcessTransforms`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ProcessEnabledMode {
    Disabled,
    ProcessesOnly,
    ContainersOnly,
}

impl ProcessEnabledMode {
    fn enables(self, key: GatedKey) -> bool {
        matches!(
            (self, key),
            (Self::ProcessesOnly, GatedKey::ProcessCollection)
                | (Self::ContainersOnly, GatedKey::ContainerCollection)
        )
    }

    /// Mirrors `loadProcessTransforms`: `ToLower` without trimming, exact `disabled`
    /// match, `ParseBool` for the true branch, everything else containers-only.
    fn parse(text: &str) -> Self {
        let lower = text.to_ascii_lowercase();
        if lower == "disabled" {
            Self::Disabled
        } else if agent_yaml::parse_bool_string(&lower).unwrap_or(false) {
            Self::ProcessesOnly
        } else {
            Self::ContainersOnly
        }
    }
}

/// Transform inputs are env and the base YAML only; it runs before `MergeFleetPolicy`.
fn legacy_process_enabled_mode(
    base_path: &str,
    yaml: &mut YamlCache,
) -> Option<ProcessEnabledMode> {
    if let Some(text) = env_string_for_key(LEGACY_PROCESS_ENABLED_KEY) {
        return Some(ProcessEnabledMode::parse(&text));
    }
    let value = yaml.value(base_path, LEGACY_PROCESS_ENABLED_KEY)?;
    // The transform keys off `IsConfigured`, so any value runs it, and a sequence or
    // mapping reaches it through `GetString` as "" once `cast.ToStringE` fails, which
    // selects containers-only. A valueless leaf is already absent here, matching
    // `loadYamlInto` dropping it rather than marking the key configured.
    Some(ProcessEnabledMode::parse(
        &agent_yaml::scalar_as_string(value).unwrap_or_default(),
    ))
}

/// Whether the user set `key` before the transform ran (`GetSource` > `SourceInfraMode`).
fn transform_time_configured(yaml: &mut YamlCache, base_path: &str, key: &str) -> bool {
    env_configured_for_key(key) || yaml.value(base_path, key).is_some()
}

fn transform_time_bool(yaml: &mut YamlCache, base_path: &str, key: &str) -> Option<bool> {
    if env_configured_for_key(key) {
        return env_bool_for_key(key);
    }
    yaml.bool_at(base_path, key)
}

/// Lazily loaded YAML files, keyed by path.
///
/// Every accessor is infallible: an unreadable or malformed file is treated as setting
/// nothing, matching the Agent, which still initializes its config from env vars and
/// schema defaults when `ReadInConfig` fails
/// (`pkg/config/nodetreemodel/read_config_file.go`).
#[derive(Default)]
pub(super) struct YamlCache(HashMap<String, Value>);

impl YamlCache {
    fn root(&mut self, path: &str) -> &Value {
        match self.0.entry(path.to_owned()) {
            Entry::Occupied(entry) => entry.into_mut(),
            Entry::Vacant(entry) => entry.insert(read_config_file(path)),
        }
    }

    /// The raw node for `key`, or `None` when the file or key is absent.
    pub(super) fn value(&mut self, path: &str, key: &str) -> Option<&Value> {
        agent_yaml::lookup_dotted_key(self.root(path), key)
    }

    /// `GetBool` for a key present in `path`; `None` when the key is not set there.
    pub(super) fn bool_at(&mut self, path: &str, key: &str) -> Option<bool> {
        // Malformed values (a mapping or sequence) coerce to false, like `cast.ToBoolE`.
        self.value(path, key)
            .map(|value| agent_yaml::value_as_bool(value).unwrap_or(false))
    }

    fn string_at(&mut self, path: &str, key: &str) -> Option<String> {
        self.value(path, key).and_then(agent_yaml::scalar_as_string)
    }

    /// Fleet policy, then env, then the gated file itself. `None` when no source sets `key`.
    pub(super) fn resolve_bool(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: &str,
    ) -> Option<bool> {
        if let Some(path) = self.fleet_policy_path(fleet_policy_file, base_path)
            && let Some(enabled) = self.bool_at(&path, key)
        {
            return Some(enabled);
        }
        if let Some(enabled) = env_bool_for_key(key) {
            return Some(enabled);
        }
        self.bool_at(base_path, key)
    }

    pub(super) fn resolve_bool_or(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: &str,
        default: bool,
    ) -> bool {
        self.resolve_bool(base_path, key, fleet_policy_file)
            .unwrap_or(default)
    }

    pub(super) fn resolve_string(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: &str,
    ) -> Option<String> {
        if let Some(path) = self.fleet_policy_path(fleet_policy_file, base_path)
            && let Some(text) = self.string_at(&path, key)
        {
            return Some(text);
        }
        if let Some(text) = env_string_for_key(key) {
            return Some(text);
        }
        self.string_at(base_path, key)
    }

    /// Env, then base YAML. Used where Go override funcs run before `MergeFleetPolicy`.
    fn resolve_string_pre_fleet(&mut self, base_path: &str, key: &str) -> Option<String> {
        if let Some(text) = env_string_for_key(key) {
            return Some(text);
        }
        self.string_at(base_path, key)
    }

    /// Whether `infrastructure_mode` selected end-user-device at the point
    /// `applyInfrastructureModeOverrides` ran, which is what decides the settings it
    /// turned on.
    ///
    /// It runs inside `LoadDatadog` before `MergeFleetPolicy` and is not re-run, so its
    /// writes survive a later fleet `infrastructure_mode` change. Contrast with
    /// [`Self::end_user_device_effective`].
    pub(super) fn end_user_device_at_load(&mut self, agent_path: &str) -> bool {
        is_end_user_device(self.resolve_string_pre_fleet(agent_path, "infrastructure_mode"))
    }

    /// The effective `infrastructure_mode` after the fleet merge, as a plain runtime
    /// `GetString("infrastructure_mode")` would see it.
    pub(super) fn end_user_device_effective(&mut self, agent_path: &str) -> bool {
        is_end_user_device(self.resolve_string(agent_path, "infrastructure_mode", AGENT_POLICY))
    }

    /// Whether `key` is explicitly set by fleet policy, env, or the gated file.
    ///
    /// Mirrors Go `IsConfigured`, used for NPM back-compat in `adjust.go`.
    pub(super) fn is_configured(
        &mut self,
        base_path: &str,
        key: &str,
        fleet_policy_file: &str,
    ) -> bool {
        if env_configured_for_key(key) {
            return true;
        }
        if let Some(path) = self.fleet_policy_path(fleet_policy_file, base_path)
            && self.value(&path, key).is_some()
        {
            return true;
        }
        self.value(base_path, key).is_some()
    }

    pub(super) fn fleet_policy_path(
        &mut self,
        filename: &str,
        config_path: &str,
    ) -> Option<String> {
        let dir = self.fleet_policies_dir(config_path)?;
        Some(
            Path::new(&dir)
                .join(filename)
                .to_string_lossy()
                .into_owned(),
        )
    }

    /// Mirrors Agent and system-probe fleet policy loading: env, then the gated config
    /// file, then the platform fallback.
    ///
    /// System-probe gates do not inherit `fleet_policies_dir` from a sibling
    /// `datadog.yaml`, matching `applyFleetPolicy` on the system-probe config object.
    fn fleet_policies_dir(&mut self, config_path: &str) -> Option<String> {
        if let Some(dir) = env_bindings::env_var_value("DD_FLEET_POLICIES_DIR") {
            return Some(dir);
        }
        if let Some(dir) = self.string_at(config_path, "fleet_policies_dir") {
            return Some(dir);
        }
        crate::platform::fleet_policies_dir_fallback().map(path_to_string)
    }

    #[cfg(test)]
    fn loaded_file_count(&self) -> usize {
        self.0.len()
    }
}

fn is_end_user_device(mode: Option<String>) -> bool {
    mode.is_some_and(|mode| mode == "end_user_device")
}

fn path_to_string(path: PathBuf) -> String {
    path.to_string_lossy().into_owned()
}

fn read_config_file(path: &str) -> Value {
    match std::fs::read_to_string(path) {
        Ok(contents) => agent_yaml::load(&contents).unwrap_or_else(|err| {
            log::warn!("condition_config_any: parse {path}: {err:#}; using env and defaults");
            Value::Null
        }),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
            log::debug!("condition_config_any: {path} not found; using env and defaults");
            Value::Null
        }
        Err(err) => {
            log::warn!("condition_config_any: read {path}: {err:#}; using env and defaults");
            Value::Null
        }
    }
}

/// Serializes and sanitizes every test that evaluates a gate.
///
/// Gate resolution reads the live process environment, and Cargo runs the lib tests as
/// threads in one process, so a test asserting that a gate stays closed has to exclude
/// the tests that set `DD_*` values even though it sets none itself. Bound variables are
/// cleared both on acquire and on drop, so tests neither inherit nor leak them.
///
/// Bind the guard to a named local, not `_`, or it drops immediately and isolates
/// nothing. Not reentrant: let one guard drop before taking another.
#[cfg(test)]
#[must_use]
pub(crate) fn test_env_guard() -> TestEnvGuard {
    static ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

    let guard = TestEnvGuard {
        _lock: ENV_LOCK.lock().unwrap_or_else(|err| err.into_inner()),
    };
    reset_test_env();
    guard
}

#[cfg(test)]
pub(crate) struct TestEnvGuard {
    _lock: std::sync::MutexGuard<'static, ()>,
}

#[cfg(test)]
impl Drop for TestEnvGuard {
    fn drop(&mut self) {
        reset_test_env();
    }
}

#[cfg(test)]
fn reset_test_env() {
    // SAFETY: reached only through TestEnvGuard, which holds ENV_LOCK.
    unsafe {
        for name in gate_env_var_names() {
            std::env::remove_var(name);
        }
    }
    set_test_agent_service_env(None);
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Agent YAML with every process-agent gate key off, including the two that default on.
    const ALL_PROCESS_GATES_OFF: &str = "\
process_config:
  process_collection:
    enabled: false
  container_collection:
    enabled: false
  process_discovery:
    enabled: false
";

    /// [`ALL_PROCESS_GATES_OFF`] plus a deprecated `process_config.enabled` value.
    ///
    /// The value has to be merged into the same `process_config` mapping: a second
    /// top-level `process_config:` block would replace the first, since duplicate keys
    /// last-win.
    fn process_gates_off_with_legacy(value: &str) -> String {
        format!(
            "\
process_config:
  enabled: {value}
  process_collection:
    enabled: false
  container_collection:
    enabled: false
  process_discovery:
    enabled: false
"
        )
    }

    /// Test fixture: holds [`test_env_guard`], gives each test a scratch config directory,
    /// and evaluates gates against a chosen [`HostOs`].
    ///
    /// The guard is not reentrant: a test must let one fixture drop before building the
    /// next, so build fixtures inside a loop body or use one per test.
    struct Gate {
        _env: TestEnvGuard,
        dir: tempfile::TempDir,
        os: HostOs,
    }

    impl Gate {
        fn new() -> Self {
            Self {
                _env: test_env_guard(),
                dir: tempfile::tempdir().unwrap(),
                os: HostOs::CURRENT,
            }
        }

        fn on(os: HostOs) -> Self {
            let mut fixture = Self::new();
            fixture.os = os;
            fixture
        }

        fn write_in(&self, dir: &Path, name: &str, body: &str) -> String {
            std::fs::create_dir_all(dir).unwrap();
            let path = dir.join(name);
            std::fs::write(&path, body).unwrap();
            path_to_string(path)
        }

        /// Writes `datadog.yaml` in the config directory and returns its path.
        fn agent(&self, body: &str) -> String {
            self.write_in(self.dir.path(), AGENT_POLICY, body)
        }

        /// Writes `system-probe.yaml` in the config directory and returns its path.
        fn sysprobe(&self, body: &str) -> String {
            self.write_in(self.dir.path(), SYSPROBE_POLICY, body)
        }

        /// Path of a file that is deliberately not created.
        fn missing(&self, name: &str) -> String {
            path_to_string(self.dir.path().join(name))
        }

        /// Writes a policy file into a named policy directory and returns that directory.
        fn policy_dir(&self, dir_name: &str, file: &str, body: &str) -> String {
            let dir = self.dir.path().join(dir_name);
            self.write_in(&dir, file, body);
            path_to_string(dir)
        }

        /// Writes a fleet policy file and points `DD_FLEET_POLICIES_DIR` at it.
        fn fleet(&self, file: &str, body: &str) -> String {
            let dir = self.policy_dir("fleet", file, body);
            self.env("DD_FLEET_POLICIES_DIR", &dir);
            dir
        }

        fn env(&self, name: &str, value: &str) {
            // SAFETY: the fixture holds the env guard for its lifetime.
            unsafe { std::env::set_var(name, value) };
        }

        /// Replaces the Agent service environment (Windows SCM `Environment`) lookup.
        fn service_env(&self, vars: &[(&str, &str)]) {
            set_test_agent_service_env(Some(
                vars.iter()
                    .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
                    .collect(),
            ));
        }

        fn met(&self, conditions: &[ConditionConfigFile]) -> bool {
            evaluate(conditions, self.os)
        }

        /// True when nothing in `conditions` vetoes the entry.
        fn veto_clear(&self, conditions: &[ConditionConfigFile]) -> bool {
            evaluate_none(conditions, self.os)
        }

        fn veto_clear_for(&self, path: &str, key: &str) -> bool {
            self.veto_clear(&[ConditionConfigFile {
                path: path.to_owned(),
                keys: vec![key.to_owned()],
            }])
        }

        fn key(&self, path: &str, key: &str) -> bool {
            self.met(&[ConditionConfigFile {
                path: path.to_owned(),
                keys: vec![key.to_owned()],
            }])
        }

        fn assert_key(&self, path: &str, key: &str, expected: bool) {
            assert_eq!(
                self.key(path, key),
                expected,
                "{key} on {:?} expected {expected}",
                self.os
            );
        }

        /// The four process-agent keys the Windows SCM check reads from `datadog.yaml`.
        fn process_gate(&self, agent: &str) -> bool {
            self.met(&[process_conditions(agent)])
        }

        /// The full process-agent gate: `datadog.yaml` keys plus system-probe keys.
        fn process_and_sysprobe_gate(&self, agent: &str, sysprobe: &str) -> bool {
            self.met(&[
                process_conditions(agent),
                ConditionConfigFile {
                    path: sysprobe.to_owned(),
                    keys: vec![NETWORK_CONFIG_KEY.into(), SYSTEM_PROBE_CONFIG_KEY.into()],
                },
            ])
        }
    }

    fn process_conditions(agent: &str) -> ConditionConfigFile {
        ConditionConfigFile {
            path: agent.to_owned(),
            keys: vec![
                LEGACY_PROCESS_ENABLED_KEY.into(),
                PROCESS_COLLECTION_KEY.into(),
                CONTAINER_COLLECTION_KEY.into(),
                PROCESS_DISCOVERY_KEY.into(),
            ],
        }
    }

    const EVERY_OS: [HostOs; 4] = [HostOs::Linux, HostOs::MacOs, HostOs::Windows, HostOs::Other];

    // ---------------------------------------------------------------- gate plumbing

    #[test]
    fn empty_conditions_are_met() {
        assert!(condition_config_any_met(&[]));
    }

    #[test]
    fn any_matching_key_enables_gate() {
        let fx = Gate::new();
        let agent = fx.agent(
            "process_config:\n  process_collection:\n    enabled: false\n  process_discovery:\n    enabled: true\n",
        );
        assert!(fx.met(&[ConditionConfigFile {
            path: agent,
            keys: vec![PROCESS_COLLECTION_KEY.into(), PROCESS_DISCOVERY_KEY.into()],
        }]));
    }

    #[test]
    fn condition_without_keys_blocks_gate() {
        let fx = Gate::new();
        let agent = fx.agent("# api_key: placeholder\n");
        assert!(!fx.met(&[ConditionConfigFile {
            path: agent,
            keys: vec![],
        }]));
    }

    #[test]
    fn unknown_key_blocks_gate() {
        let fx = Gate::new();
        let agent = fx.agent("not_a_gate:\n  enabled: true\n");
        fx.assert_key(&agent, "not_a_gate.enabled", false);
    }

    #[test]
    fn missing_file_falls_back_to_defaults() {
        let fx = Gate::new();
        let missing = fx.missing("datadog.yaml");
        fx.assert_key(&missing, LEGACY_PROCESS_ENABLED_KEY, false);
        fx.assert_key(&missing, PROCESS_COLLECTION_KEY, false);
        // Defaults that are on stay on when the file is absent.
        fx.assert_key(&missing, CONTAINER_COLLECTION_KEY, true);
    }

    /// `ReadInConfig` reports a parse failure as `ErrConfigFileNotFound`, which
    /// `comp/core/config/setup.go` swallows unless `-c` named the file. The core Agent
    /// service is registered without `-c`, so an unparseable `datadog.yaml` leaves the
    /// Agent running on env and defaults and the SCM check still starts process-agent.
    #[test]
    fn unparseable_file_falls_back_to_defaults() {
        // A top-level scalar is the Go TestReadInConfigExactError case; the unclosed flow
        // sequence fails in the event stream instead.
        for body in ["site:datadoghq.eu\n", "process_config: [unclosed\n"] {
            assert!(agent_yaml::load(body).is_err(), "{body:?} should not parse");
            let fx = Gate::new();
            let agent = fx.agent(body);
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
            fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, true);
            assert!(
                fx.process_gate(&agent),
                "{body:?} should leave the gate open"
            );
        }
    }

    #[test]
    fn stock_config_uses_agent_defaults() {
        let fx = Gate::new();
        let agent = fx.agent("# api_key: placeholder\n");
        assert!(fx.process_gate(&agent));
    }

    #[test]
    fn all_false_keys_block_gate() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        assert!(!fx.process_gate(&agent));
    }

    #[test]
    fn yaml_cache_reads_each_path_once() {
        let fx = Gate::new();
        let path = fx.agent(ALL_PROCESS_GATES_OFF);

        let mut cache = YamlCache::default();
        for key in [PROCESS_COLLECTION_KEY, PROCESS_DISCOVERY_KEY] {
            cache.bool_at(&path, key);
        }
        assert_eq!(cache.loaded_file_count(), 1);
    }

    #[test]
    fn path_expands_env_vars() {
        let fx = Gate::new();
        let conf_dir = fx.policy_dir(
            "agent-conf",
            AGENT_POLICY,
            "process_config:\n  process_collection:\n    enabled: true\n",
        );
        fx.env("DD_CONF_DIR", &conf_dir);
        fx.assert_key("${DD_CONF_DIR}/datadog.yaml", PROCESS_COLLECTION_KEY, true);
    }

    #[test]
    fn empty_veto_is_clear() {
        assert!(condition_config_none_met(&[]));
    }

    #[test]
    fn veto_fires_when_its_key_is_true() {
        let fx = Gate::new();
        let sysprobe = fx.sysprobe("system_probe_config:\n  external: true\n");
        assert!(!fx.veto_clear_for(&sysprobe, SYSTEM_PROBE_EXTERNAL_KEY));
    }

    /// Both ways of not setting it: written false, and absent so the schema default of
    /// false applies. An install that never heard of the key must not be vetoed.
    #[test]
    fn veto_is_clear_when_its_key_is_false_or_absent() {
        for body in [
            "system_probe_config:\n  external: false\n",
            "# nothing set\n",
        ] {
            let fx = Gate::new();
            let sysprobe = fx.sysprobe(body);
            assert!(
                fx.veto_clear_for(&sysprobe, SYSTEM_PROBE_EXTERNAL_KEY),
                "vetoed on {body:?}"
            );
        }
    }

    /// An unknown key resolves false, so it cannot veto. That is the safe direction: a
    /// key the daemon does not understand must not be what stops a workload running.
    #[test]
    fn unknown_veto_key_leaves_the_veto_clear() {
        let fx = Gate::new();
        let sysprobe = fx.sysprobe("not_a_gate:\n  enabled: true\n");
        assert!(fx.veto_clear_for(&sysprobe, "not_a_gate.enabled"));
    }

    /// `startSystemProbe` returns ErrNotEnabled when `external` is set, whatever the
    /// modules say, so the derived key has to agree.
    #[test]
    fn external_system_probe_closes_the_derived_key() {
        let fx = Gate::new();
        let sysprobe = fx
            .sysprobe("system_probe_config:\n  external: true\nnetwork_config:\n  enabled: true\n");
        fx.assert_key(&sysprobe, SYSTEM_PROBE_CONFIG_KEY, false);
    }

    #[test]
    fn env_external_system_probe_closes_the_derived_key() {
        let fx = Gate::new();
        let sysprobe = fx.sysprobe("network_config:\n  enabled: true\n");
        fx.env("DD_SYSTEM_PROBE_EXTERNAL", "true");
        fx.assert_key(&sysprobe, SYSTEM_PROBE_CONFIG_KEY, false);
    }

    /// The blast radius of the fold-in. `derived_enabled` backs the process-agent gate
    /// too, and process-agent has to keep running against an external system-probe, so
    /// the literal keys beside the derived one must stay open. That is also why the veto
    /// has to be per entry rather than per key.
    #[test]
    fn external_system_probe_leaves_the_network_key_open() {
        let fx = Gate::new();
        let sysprobe = fx
            .sysprobe("system_probe_config:\n  external: true\nnetwork_config:\n  enabled: true\n");
        fx.assert_key(&sysprobe, NETWORK_CONFIG_KEY, true);
    }

    #[test]
    fn summary_lists_every_path_and_key() {
        assert_eq!(
            condition_config_summary(&[
                ConditionConfigFile {
                    path: "/etc/datadog-agent/datadog.yaml".into(),
                    keys: vec![
                        LEGACY_PROCESS_ENABLED_KEY.into(),
                        PROCESS_COLLECTION_KEY.into()
                    ],
                },
                ConditionConfigFile {
                    path: "/etc/datadog-agent/system-probe.yaml".into(),
                    keys: vec![NETWORK_CONFIG_KEY.into()],
                },
            ]),
            "/etc/datadog-agent/datadog.yaml:process_config.enabled, \
             /etc/datadog-agent/datadog.yaml:process_config.process_collection.enabled, \
             /etc/datadog-agent/system-probe.yaml:network_config.enabled"
        );
    }

    // --------------------------------------------------------------- YAML semantics

    #[test]
    fn yaml_scalar_spelling_decides_the_gate() {
        // Plain scalars resolve like Go yaml.v2, quoted ones go through ParseBool only.
        for (value, expected) in [
            ("true", true),
            ("yes", true),
            ("on", true),
            ("!!bool yes", true),
            ("!!str yes", false),
            ("\"yes\"", false),
            ("1.0", true),
            (".inf", true),
            ("18446744073709551615", true),
            ("0x1", true),
            ("\"1.0\"", false),
            ("\"0x1\"", false),
            ("not-a-bool", false),
            ("[]", false),
        ] {
            let fx = Gate::new();
            let agent = fx.agent(&format!(
                "process_config:\n  process_discovery:\n    enabled: {value}\n"
            ));
            fx.assert_key(&agent, PROCESS_DISCOVERY_KEY, expected);
        }
    }

    #[test]
    fn flattened_and_mixed_case_keys_enable_gate() {
        for body in [
            "process_config.process_collection.enabled: true\n",
            "process_config:\n  Process_Collection:\n    enabled: true\n",
            "process_config:\n  process_collection.enabled: true\n",
            "process_config.process_collection:\n  enabled: true\n",
        ] {
            let fx = Gate::new();
            let agent = fx.agent(body);
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
        }
    }

    #[test]
    fn duplicate_key_last_value_wins() {
        let fx = Gate::new();
        let agent = fx.agent(
            "process_config:\n  process_collection:\n    enabled: false\nprocess_config:\n  process_collection:\n    enabled: true\n",
        );
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
    }

    #[test]
    fn multi_document_yaml_uses_first_document() {
        let fx = Gate::new();
        let agent = fx.agent(
            "process_config:\n  process_collection:\n    enabled: true\n---\nprocess_config:\n  process_collection:\n    enabled: false\n",
        );
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
    }

    #[test]
    fn invalid_yaml_falls_back_to_env_and_defaults() {
        let fx = Gate::new();
        let agent = fx.agent("{\n");
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, true);
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);

        fx.env("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true");
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
    }

    // ------------------------------------------------- deprecated process_config.enabled

    /// `loadProcessTransforms` normalization, from YAML and from env.
    #[test]
    fn legacy_process_enabled_normalizes_collection_keys() {
        // (value, process collection, container collection)
        let cases = [
            ("true", true, false),
            ("false", false, true),
            ("disabled", false, false),
            ("1", true, false),
            ("0", false, true),
            // GetString("-0x1") is "-1", which ParseBool rejects, so containers-only.
            ("-0x1", false, true),
        ];

        for (value, process, container) in cases {
            let fx = Gate::new();
            let agent = fx.agent(&format!(
                "process_config:\n  enabled: {value}\n  process_discovery:\n    enabled: false\n"
            ));
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, process);
            fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, container);
            // The deprecated key itself is rewritten to the process-collection value.
            fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, process);
        }

        for (value, process, container) in cases {
            let fx = Gate::new();
            let agent = fx.agent("process_config:\n  process_discovery:\n    enabled: false\n");
            fx.env("DD_PROCESS_CONFIG_ENABLED", value);
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, process);
            fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, container);
            fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, process);
        }
    }

    /// Whitespace is not trimmed, so a padded value is neither `disabled` nor a bool,
    /// which lands on containers-only.
    #[test]
    fn legacy_process_enabled_yaml_does_not_trim_whitespace() {
        let fx = Gate::new();
        let agent = fx.agent(
            "process_config:\n  enabled: \" disabled \"\n  process_discovery:\n    enabled: false\n",
        );
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, true);
    }

    #[test]
    fn legacy_process_enabled_env_does_not_trim_whitespace() {
        let fx = Gate::new();
        let agent = fx.agent("process_config:\n  process_discovery:\n    enabled: false\n");
        fx.env("DD_PROCESS_CONFIG_ENABLED", " disabled ");
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, true);
    }

    /// The transform keys off `IsConfigured`, so a sequence or mapping runs it too, and
    /// reaches it through `GetString` as "", which is neither `disabled` nor a bool. The
    /// infrastructure mode is set so that skipping the transform would be visible: it
    /// would leave process collection unconfigured for `end_user_device` to enable.
    #[test]
    fn legacy_process_enabled_non_scalar_runs_the_transform() {
        for value in ["[1, 2]", "{a: b}"] {
            let fx = Gate::new();
            let agent = fx.agent(&format!(
                "infrastructure_mode: end_user_device\nprocess_config:\n  enabled: {value}\n  process_discovery:\n    enabled: false\n"
            ));
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
            fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, true);
            fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, false);
        }
    }

    /// Mirrors `TestProcConfigEnabledTransformPrecedence` in pkg/config/setup/process_test.go.
    #[test]
    fn explicit_collection_keys_win_over_legacy_process_enabled() {
        // (legacy, process collection env, container collection env, expected p/c/legacy)
        for (legacy, process_env, container_env, expected) in [
            ("false", None, Some("false"), (false, false, false)),
            ("true", Some("false"), None, (false, false, false)),
            ("disabled", Some("true"), Some("true"), (true, true, true)),
            // The deprecated key still fills a collection key the user left unset.
            ("false", Some("true"), None, (true, true, true)),
        ] {
            let fx = Gate::new();
            let agent = fx.agent("# api_key: placeholder\n");
            fx.env("DD_PROCESS_CONFIG_ENABLED", legacy);
            if let Some(value) = process_env {
                fx.env("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", value);
            }
            if let Some(value) = container_env {
                fx.env("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", value);
            }

            let (process, container, normalized) = expected;
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, process);
            fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, container);
            fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, normalized);
        }
    }

    #[test]
    fn explicit_yaml_collection_keys_win_over_legacy_process_enabled() {
        let fx = Gate::new();
        let agent = fx.agent(&process_gates_off_with_legacy("1"));
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, false);
        assert!(
            !fx.process_gate(&agent),
            "explicit collection/discovery false must win over process_config.enabled"
        );
    }

    // ------------------------------------------------------------ infrastructure_mode

    #[test]
    fn end_user_device_enables_process_collection_when_unset() {
        for source in ["yaml", "env"] {
            let fx = Gate::new();
            let agent = match source {
                "yaml" => fx.agent(
                    "infrastructure_mode: end_user_device\nprocess_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
                ),
                _ => {
                    let agent = fx.agent("# api_key: placeholder\n");
                    fx.env("DD_INFRASTRUCTURE_MODE", "end_user_device");
                    fx.env("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
                    fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");
                    agent
                }
            };
            fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
        }
    }

    #[test]
    fn explicit_process_collection_wins_over_end_user_device() {
        let fx = Gate::new();
        let agent = fx.agent(&format!(
            "infrastructure_mode: end_user_device\n{ALL_PROCESS_GATES_OFF}"
        ));
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
    }

    #[test]
    fn legacy_transform_overrides_end_user_device() {
        let fx = Gate::new();
        let agent = fx.agent(
            "infrastructure_mode: end_user_device\nprocess_config:\n  enabled: disabled\n  process_discovery:\n    enabled: false\n",
        );
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
    }

    /// `applyInfrastructureModeOverrides` runs before the fleet merge, so a fleet-only
    /// mode never enables process collection.
    #[test]
    fn fleet_end_user_device_does_not_enable_process_collection() {
        let fx = Gate::new();
        fx.fleet(
            AGENT_POLICY,
            "infrastructure_mode: end_user_device\nprocess_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
        );
        let agent = fx.agent(
            "process_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
        );
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
        assert!(!fx.process_gate(&agent));
    }

    // ------------------------------------------------------------------ env bindings

    #[test]
    fn env_can_flip_defaults_either_way() {
        let fx = Gate::new();
        let agent = fx.agent("# api_key: placeholder\n");
        fx.env("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false");
        fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");
        assert!(!fx.process_gate(&agent));

        fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "true");
        assert!(fx.process_gate(&agent));
    }

    #[test]
    fn env_values_go_through_parse_bool_only() {
        for (value, expected) in [
            ("true", true),
            ("1", true),
            ("yes", false),
            (" true ", false),
        ] {
            let fx = Gate::new();
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", value);
            fx.assert_key(&agent, PROCESS_DISCOVERY_KEY, expected);
        }
    }

    #[test]
    fn empty_env_value_counts_as_unset() {
        // Matches `os.LookupEnv(..); ok && value != ""` in nodetreemodel.
        let fx = Gate::new();
        let agent = fx.agent("process_config:\n  enabled: disabled\n");
        fx.env("DD_PROCESS_CONFIG_ENABLED", "");
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, false);

        fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "");
        assert_eq!(env_bool_for_key(PROCESS_DISCOVERY_KEY), None);
        assert!(!env_configured_for_key(PROCESS_DISCOVERY_KEY));
    }

    #[test]
    fn empty_env_value_falls_through_to_next_bound_var() {
        let fx = Gate::new();
        fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "");
        fx.env("DD_PROCESS_CONFIG_DISCOVERY_ENABLED", "true");
        assert_eq!(env_bool_for_key(PROCESS_DISCOVERY_KEY), Some(true));
        assert!(env_configured_for_key(PROCESS_DISCOVERY_KEY));
    }

    #[test]
    fn empty_env_value_falls_through_for_the_legacy_transform() {
        let fx = Gate::new();
        let agent = fx.agent("process_config:\n  enabled: false\n");
        fx.env("DD_PROCESS_CONFIG_ENABLED", "");
        fx.env("DD_PROCESS_AGENT_ENABLED", "disabled");
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, false);
    }

    // ------------------------------------------- Agent service environment (Windows SCM)

    #[test]
    fn service_env_is_used_when_process_env_is_unset() {
        let fx = Gate::new();
        let agent = fx.agent("# api_key: placeholder\n");
        fx.service_env(&[
            ("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false"),
            ("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false"),
        ]);
        assert!(!fx.process_gate(&agent));
    }

    #[test]
    fn service_env_wins_over_process_env() {
        let fx = Gate::new();
        fx.service_env(&[("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false")]);
        fx.env("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "true");
        assert_eq!(env_bool_for_key(CONTAINER_COLLECTION_KEY), Some(false));
    }

    #[test]
    fn service_env_is_matched_case_insensitively_and_ignores_empty() {
        let fx = Gate::new();
        fx.service_env(&[
            ("dd_process_config_container_collection_enabled", "false"),
            ("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", ""),
        ]);
        assert_eq!(env_bool_for_key(CONTAINER_COLLECTION_KEY), Some(false));
        assert_eq!(env_bool_for_key(PROCESS_DISCOVERY_KEY), None);
    }

    /// The SCM merges the service block over the inherited environment, so clearing a
    /// machine-level `DD_*` there leaves the Agent with an empty value, which its env
    /// layer skips. procmgr must not reach past the cleared entry to the machine value.
    #[test]
    fn empty_service_env_shadows_the_process_env() {
        let fx = Gate::new();
        fx.service_env(&[("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "")]);
        fx.env("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true");
        assert_eq!(env_bool_for_key(PROCESS_COLLECTION_KEY), None);

        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
    }

    #[test]
    fn service_env_drives_the_legacy_transform() {
        let fx = Gate::new();
        let agent = fx.agent("# api_key: placeholder\n");
        fx.service_env(&[
            ("DD_PROCESS_CONFIG_ENABLED", "disabled"),
            ("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", "false"),
            ("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false"),
        ]);
        assert!(!fx.process_gate(&agent));
    }

    #[test]
    fn service_env_can_name_the_fleet_policies_dir() {
        let fx = Gate::new();
        let dir = fx.policy_dir(
            "fleet",
            AGENT_POLICY,
            "process_config:\n  process_collection:\n    enabled: true\n",
        );
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        fx.service_env(&[("DD_FLEET_POLICIES_DIR", &dir)]);
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
    }

    #[test]
    fn service_env_values_are_not_trimmed() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        fx.service_env(&[("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", " true ")]);
        assert!(!fx.process_gate(&agent));
    }

    // -------------------------------------------------------------------- fleet policy

    #[test]
    fn fleet_policy_overrides_file_and_env() {
        let fx = Gate::new();
        fx.fleet(
            AGENT_POLICY,
            "process_config:\n  process_discovery:\n    enabled: true\n",
        );
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        fx.env("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", "false");
        fx.assert_key(&agent, PROCESS_DISCOVERY_KEY, true);
    }

    #[test]
    fn fleet_policy_can_disable_a_default_enabled_key() {
        let fx = Gate::new();
        fx.fleet(
            AGENT_POLICY,
            "process_config:\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n",
        );
        let agent = fx.agent("# api_key: placeholder\n");
        assert!(!fx.process_gate(&agent));
    }

    #[test]
    fn fleet_policy_applies_when_the_local_file_is_missing() {
        for (policy, key) in [
            ("network_config:\n  enabled: true\n", NETWORK_CONFIG_KEY),
            (
                "system_probe_config:\n  enabled: true\n",
                SYSTEM_PROBE_CONFIG_KEY,
            ),
        ] {
            let fx = Gate::new();
            fx.fleet(SYSPROBE_POLICY, policy);
            let sysprobe = fx.missing(SYSPROBE_POLICY);
            fx.assert_key(&sysprobe, key, true);
        }
    }

    #[test]
    fn fleet_policies_dir_can_come_from_datadog_yaml() {
        let fx = Gate::new();
        let dir = fx.policy_dir(
            "fleet",
            AGENT_POLICY,
            "process_config:\n  process_collection:\n    enabled: true\n",
        );
        let agent = fx.agent(&format!(
            "fleet_policies_dir: {dir}\n{ALL_PROCESS_GATES_OFF}"
        ));
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, true);
    }

    #[test]
    fn fleet_policies_dir_can_come_from_system_probe_yaml() {
        let fx = Gate::new();
        let dir = fx.policy_dir(
            "fleet",
            SYSPROBE_POLICY,
            "network_config:\n  enabled: true\n",
        );
        let sysprobe = fx.sysprobe(&format!(
            "fleet_policies_dir: {dir}\nnetwork_config:\n  enabled: false\n"
        ));
        fx.assert_key(&sysprobe, NETWORK_CONFIG_KEY, true);
    }

    /// `applyFleetPolicy` reads `fleet_policies_dir` from the config object being
    /// loaded, so a system-probe gate ignores the sibling `datadog.yaml` value.
    #[test]
    fn system_probe_gate_ignores_fleet_policies_dir_from_datadog_yaml() {
        let fx = Gate::new();
        let sysprobe_dir = fx.policy_dir(
            "fleet",
            SYSPROBE_POLICY,
            "network_config:\n  enabled: true\n",
        );
        let agent_dir = fx.policy_dir(
            "other-fleet",
            SYSPROBE_POLICY,
            "network_config:\n  enabled: false\n",
        );
        fx.agent(&format!("fleet_policies_dir: {agent_dir}\n"));
        let sysprobe = fx.sysprobe(&format!(
            "fleet_policies_dir: {sysprobe_dir}\nnetwork_config:\n  enabled: false\n"
        ));
        fx.assert_key(&sysprobe, NETWORK_CONFIG_KEY, true);
    }

    /// Without the Windows registry fallback there is no other source for the directory.
    #[test]
    #[cfg(not(windows))]
    fn system_probe_gate_without_its_own_fleet_dir_has_no_policy() {
        let fx = Gate::new();
        let dir = fx.policy_dir(
            "fleet",
            SYSPROBE_POLICY,
            "network_config:\n  enabled: true\n",
        );
        fx.agent(&format!("fleet_policies_dir: {dir}\n"));
        let sysprobe = fx.sysprobe("network_config:\n  enabled: false\n");
        fx.assert_key(&sysprobe, NETWORK_CONFIG_KEY, false);
    }

    /// The transform runs before `MergeFleetPolicy`, so fleet cannot rewrite collection
    /// keys, and cannot restore a deprecated key the transform already normalized.
    #[test]
    fn fleet_legacy_process_enabled_does_not_run_the_transform() {
        let fx = Gate::new();
        fx.fleet(AGENT_POLICY, "process_config:\n  enabled: true\n");
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
        fx.assert_key(&agent, CONTAINER_COLLECTION_KEY, false);
        // The deprecated key itself still honors fleet policy.
        fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, true);
    }

    /// Once the transform normalizes the deprecated key, fleet policy cannot restore it:
    /// the transform writes at `SourceAgentRuntime`.
    #[test]
    fn fleet_cannot_restore_a_normalized_legacy_process_enabled() {
        let fx = Gate::new();
        fx.fleet(AGENT_POLICY, "process_config:\n  enabled: true\n");
        let agent = fx.agent(&process_gates_off_with_legacy("true"));
        fx.assert_key(&agent, LEGACY_PROCESS_ENABLED_KEY, false);
        assert!(!fx.process_gate(&agent));
    }

    /// Env still drives the transform, and the value it writes outranks fleet policy.
    #[test]
    fn env_legacy_transform_outranks_fleet_policy() {
        let fx = Gate::new();
        fx.fleet(AGENT_POLICY, "process_config:\n  enabled: true\n");
        let agent = fx.agent("process_config:\n  process_discovery:\n    enabled: false\n");
        fx.env("DD_PROCESS_CONFIG_ENABLED", "false");
        fx.assert_key(&agent, PROCESS_COLLECTION_KEY, false);
    }

    // ------------------------------------------------ derived system_probe_config.enabled

    #[test]
    fn system_probe_gate_needs_a_module_or_fleet_policy() {
        let fx = Gate::new();
        // The Linux schema default would otherwise enable discovery with no YAML at all.
        fx.env("DD_DISCOVERY_ENABLED", "false");
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.missing(SYSPROBE_POLICY);
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    #[test]
    fn local_network_config_enables_system_probe_gate() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.sysprobe("network_config:\n  enabled: true\n");
        assert!(fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    #[test]
    fn env_network_config_enables_system_probe_gate_without_a_local_file() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.missing(SYSPROBE_POLICY);
        fx.env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true");
        assert!(fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    #[test]
    fn single_knob_modules_enable_system_probe_gate() {
        for body in [
            "system_probe_config:\n  enable_tcp_queue_length: true\n",
            // A module beats an explicit `system_probe_config.enabled: false`, because the
            // runtime value is derived from the enabled module set.
            "system_probe_config:\n  enabled: false\n  enable_oom_kill: true\n",
            "system_probe_config:\n  process_config:\n    enabled: true\n",
            "ebpf_check:\n  enabled: true\n",
            "ping:\n  enabled: true\n",
            "traceroute:\n  enabled: true\n",
            "privileged_logs:\n  enabled: true\n",
            "noisy_neighbor:\n  enabled: true\n",
            "windows_crash_detection:\n  enabled: true\n",
            "gpu_monitoring:\n  enabled: true\n",
            "dynamic_instrumentation:\n  enabled: true\n",
            "runtime_security_config:\n  enabled: true\n",
            "runtime_security_config:\n  fim_enabled: true\n",
            "compliance_config:\n  database_benchmarks:\n    enabled: true\n",
        ] {
            let fx = Gate::new();
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe(&format!("discovery:\n  enabled: false\n{body}"));
            assert!(
                fx.process_and_sysprobe_gate(&agent, &sysprobe),
                "expected system-probe gate open for:\n{body}"
            );
        }
    }

    /// `adjust.go`: `system_probe_config.enabled: true` with no NPM or USM setting
    /// enables NPM for back-compat.
    #[test]
    fn system_probe_enabled_back_compat_enables_npm() {
        for (body, expected) in [
            ("system_probe_config:\n  enabled: true\n", true),
            (
                "system_probe_config:\n  enabled: true\nservice_monitoring_config:\n  enabled: false\n",
                true,
            ),
            // A valueless key is not configured, so back-compat still applies.
            (
                "system_probe_config:\n  enabled: true\nnetwork_config:\n  enabled:\nservice_monitoring_config:\n  enabled: false\n",
                true,
            ),
            // An explicit NPM value opts out of back-compat.
            (
                "system_probe_config:\n  enabled: true\nnetwork_config:\n  enabled: false\n",
                false,
            ),
        ] {
            let fx = Gate::new();
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe(&format!("discovery:\n  enabled: false\n{body}"));
            assert_eq!(
                fx.process_and_sysprobe_gate(&agent, &sysprobe),
                expected,
                "back-compat for:\n{body}"
            );
        }
    }

    #[test]
    fn env_npm_disable_blocks_back_compat() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe =
            fx.sysprobe("system_probe_config:\n  enabled: true\ndiscovery:\n  enabled: false\n");
        fx.env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "false");
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    #[test]
    fn fleet_npm_disable_blocks_back_compat() {
        let fx = Gate::new();
        fx.fleet(SYSPROBE_POLICY, "network_config:\n  enabled: false\n");
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe =
            fx.sysprobe("system_probe_config:\n  enabled: true\ndiscovery:\n  enabled: false\n");
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    /// Env beats the local `service_monitoring_config.enabled: false`.
    #[test]
    fn env_usm_enables_system_probe_gate() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.sysprobe("service_monitoring_config:\n  enabled: false\n");
        fx.env("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true");
        assert!(fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    /// adjust_npm.go: the sk tracer clears USM, so with nothing else on the gate closes.
    #[test]
    fn sk_tracer_clears_usm_for_the_system_probe_gate() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.sysprobe(
            "service_monitoring_config:\n  enabled: true\nnetwork_config:\n  enable_sk_tracer: true\ndiscovery:\n  enabled: false\n",
        );
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    /// The sk tracer needs CO-RE and ring buffers, so disabling CO-RE keeps USM on.
    #[test]
    fn sk_tracer_without_co_re_keeps_usm() {
        let fx = Gate::new();
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.sysprobe(
            "service_monitoring_config:\n  enabled: true\nnetwork_config:\n  enable_sk_tracer: true\n",
        );
        fx.env("DD_ENABLE_CO_RE", "false");
        assert!(fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    #[test]
    fn fleet_policy_beats_env_for_a_module_toggle() {
        let fx = Gate::new();
        fx.fleet(
            SYSPROBE_POLICY,
            "service_monitoring_config:\n  enabled: false\n",
        );
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
        fx.env("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true");
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    /// adjust_discovery.go: full USM, the sk tracer, and ebpfless each make the discovery
    /// service map redundant.
    #[test]
    fn discovery_service_map_is_dropped_when_redundant() {
        for body in [
            "service_monitoring_config:\n  enabled: false\nnetwork_config:\n  enable_ebpfless: true\n",
            "service_monitoring_config:\n  enabled: false\nnetwork_config:\n  enable_sk_tracer: true\n",
        ] {
            let fx = Gate::new();
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe(&format!(
                "discovery:\n  enabled: false\n  service_map:\n    enabled: true\n{body}"
            ));
            assert!(
                !fx.process_and_sysprobe_gate(&agent, &sysprobe),
                "expected service map to be dropped for:\n{body}"
            );
        }
    }

    #[test]
    fn malformed_module_value_does_not_block_derivation() {
        let fx = Gate::new();
        let sysprobe = fx.sysprobe(
            "network_config:\n  enabled: []\nservice_monitoring_config:\n  enabled: true\n",
        );
        fx.assert_key(&sysprobe, SYSTEM_PROBE_CONFIG_KEY, true);
    }

    // ------------------------------------------------------- platform-specific modules

    /// `discovery.enabled` defaults on for Linux outside Fargate and off elsewhere.
    #[test]
    fn discovery_platform_default_decides_an_empty_system_probe_config() {
        for os in EVERY_OS {
            let fx = Gate::on(os);
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe("# empty\n");
            assert_eq!(
                fx.process_and_sysprobe_gate(&agent, &sysprobe),
                os == HostOs::Linux
            );
        }

        for (name, value) in [
            ("ECS_FARGATE", "true"),
            ("AWS_EXECUTION_ENV", "AWS_ECS_FARGATE"),
        ] {
            let fx = Gate::on(HostOs::Linux);
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe("# empty\n");
            fx.env(name, value);
            assert!(
                !fx.process_and_sysprobe_gate(&agent, &sysprobe),
                "Fargate ({name}) must not inherit the Linux discovery default"
            );
        }
    }

    #[test]
    fn explicit_discovery_disable_beats_the_linux_default() {
        for body in ["discovery:\n  enabled: false\n", "# empty\n"] {
            let fx = Gate::on(HostOs::Linux);
            let agent = fx.agent(ALL_PROCESS_GATES_OFF);
            let sysprobe = fx.sysprobe(body);
            if body.starts_with('#') {
                fx.env("DD_DISCOVERY_ENABLED", "false");
            }
            assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
        }
    }

    /// `logon_duration` and `notable_events` are macOS-only modules read from the core
    /// `datadog.yaml`, and `software_inventory` is Windows and macOS only.
    #[test]
    fn core_agent_modules_follow_the_go_os_gates() {
        for (body, enabled_on) in [
            ("notable_events:\n  enabled: true\n", &[HostOs::MacOs][..]),
            ("logon_duration:\n  enabled: true\n", &[HostOs::MacOs][..]),
            (
                "software_inventory:\n  enabled: true\n",
                &[HostOs::MacOs, HostOs::Windows][..],
            ),
        ] {
            for os in EVERY_OS {
                let fx = Gate::on(os);
                let agent = fx.agent(&format!("{ALL_PROCESS_GATES_OFF}{body}"));
                let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
                assert_eq!(
                    fx.process_and_sysprobe_gate(&agent, &sysprobe),
                    enabled_on.contains(&os),
                    "os={os:?} for:\n{body}"
                );
            }
        }
    }

    #[test]
    fn macos_core_agent_modules_are_ignored_in_system_probe_yaml() {
        let fx = Gate::on(HostOs::MacOs);
        let agent = fx.agent(ALL_PROCESS_GATES_OFF);
        let sysprobe =
            fx.sysprobe("discovery:\n  enabled: false\nlogon_duration:\n  enabled: true\n");
        assert!(!fx.process_and_sysprobe_gate(&agent, &sysprobe));
    }

    /// `applyInfrastructureModeOverrides` runs before the fleet merge and is not re-run,
    /// so the settings it enabled survive a fleet `infrastructure_mode` change, while a
    /// fleet value for one of those settings still wins.
    #[test]
    fn end_user_device_module_defaults_survive_a_fleet_mode_change() {
        for os in EVERY_OS {
            let fx = Gate::on(os);
            fx.fleet(AGENT_POLICY, "infrastructure_mode: full\n");
            let agent = fx.agent(&format!(
                "infrastructure_mode: end_user_device\n{ALL_PROCESS_GATES_OFF}"
            ));
            let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
            assert_eq!(
                fx.process_and_sysprobe_gate(&agent, &sysprobe),
                matches!(os, HostOs::MacOs | HostOs::Windows),
                "os={os:?}"
            );
        }

        for (os, expected) in [
            // macOS keeps the gate open through notable_events.
            (HostOs::MacOs, true),
            (HostOs::Windows, false),
            (HostOs::Linux, false),
        ] {
            let fx = Gate::on(os);
            fx.fleet(
                AGENT_POLICY,
                "infrastructure_mode: full\nsoftware_inventory:\n  enabled: false\n",
            );
            let agent = fx.agent(&format!(
                "infrastructure_mode: end_user_device\n{ALL_PROCESS_GATES_OFF}"
            ));
            let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
            assert_eq!(
                fx.process_and_sysprobe_gate(&agent, &sysprobe),
                expected,
                "os={os:?}"
            );
        }
    }

    /// config.go reads `infrastructure_mode` at runtime, so a fleet policy that moves the
    /// host out of end-user-device mode also drops the network tracer that mode implied.
    /// The settings the mode wrote at load time are unaffected, which is why this differs
    /// from [`end_user_device_module_defaults_survive_a_fleet_mode_change`].
    #[test]
    fn fleet_mode_change_drops_the_end_user_device_network_tracer() {
        let fx = Gate::on(HostOs::Linux);
        fx.fleet(AGENT_POLICY, "infrastructure_mode: full\n");
        fx.agent(&format!(
            "infrastructure_mode: end_user_device\n{ALL_PROCESS_GATES_OFF}"
        ));
        let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
        fx.assert_key(&sysprobe, SYSTEM_PROBE_CONFIG_KEY, false);
    }

    /// End-user-device mode enables NPM through the network tracer module.
    #[test]
    fn end_user_device_enables_system_probe_gate() {
        for os in EVERY_OS {
            let fx = Gate::on(os);
            let agent = fx.agent(&format!(
                "infrastructure_mode: end_user_device\n{ALL_PROCESS_GATES_OFF}"
            ));
            let sysprobe = fx.sysprobe("discovery:\n  enabled: false\n");
            fx.assert_key(&sysprobe, SYSTEM_PROBE_CONFIG_KEY, true);
            assert!(fx.process_and_sysprobe_gate(&agent, &sysprobe));
        }
    }

    // --------------------------------------------- system-probe catalog entry keys

    /// The three keys the system-probe processes.d entry adds beyond the process-agent
    /// ones must resolve, not fall through to the unknown-key branch, which returns false
    /// and warns on every evaluation.
    #[test]
    fn system_probe_catalog_keys_resolve() {
        for (body, key, in_sysprobe) in [
            (
                "windows_crash_detection:\n  enabled: true\n",
                WINDOWS_CRASH_DETECTION_KEY,
                true,
            ),
            (
                "runtime_security_config:\n  enabled: true\n",
                RUNTIME_SECURITY_KEY,
                true,
            ),
            (
                "software_inventory:\n  enabled: true\n",
                SOFTWARE_INVENTORY_KEY,
                false,
            ),
        ] {
            let fx = Gate::new();
            let path = if in_sysprobe {
                fx.sysprobe(body)
            } else {
                fx.agent(body)
            };
            fx.assert_key(&path, key, true);
        }
    }

    #[test]
    fn system_probe_catalog_keys_default_off() {
        let fx = Gate::new();
        let sysprobe = fx.sysprobe("# empty\n");
        let agent = fx.agent("# empty\n");
        fx.assert_key(&sysprobe, WINDOWS_CRASH_DETECTION_KEY, false);
        fx.assert_key(&sysprobe, RUNTIME_SECURITY_KEY, false);
        fx.assert_key(&agent, SOFTWARE_INVENTORY_KEY, false);
    }

    /// `software_inventory.enabled` is one of the settings
    /// `applyInfrastructureModeOverrides` turns on, so the key follows the mode when no
    /// source sets it, exactly as the derived key already does.
    #[test]
    fn end_user_device_enables_the_software_inventory_key() {
        // One fixture at a time: the env guard is not reentrant, and two live in the same
        // scope would deadlock rather than fail.
        for (body, expected) in [
            ("infrastructure_mode: end_user_device\n", true),
            (
                "infrastructure_mode: end_user_device\nsoftware_inventory:\n  enabled: false\n",
                false,
            ),
        ] {
            let fx = Gate::new();
            let agent = fx.agent(body);
            fx.assert_key(&agent, SOFTWARE_INVENTORY_KEY, expected);
        }
    }

    #[test]
    fn fleet_policy_drives_the_system_probe_catalog_keys() {
        let fx = Gate::new();
        fx.fleet(
            SYSPROBE_POLICY,
            "runtime_security_config:\n  enabled: true\n",
        );
        let sysprobe = fx.sysprobe("runtime_security_config:\n  enabled: false\n");
        fx.assert_key(&sysprobe, RUNTIME_SECURITY_KEY, true);
    }
}
