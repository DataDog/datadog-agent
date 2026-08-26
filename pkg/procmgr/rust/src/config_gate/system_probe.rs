// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Derived `system_probe_config.enabled` for process-manager config gates.
//!
//! # Keep in sync with Go
//!
//! Mirrors `load()` in `pkg/system-probe/config/config.go` and the NPM back-compat
//! rule in `pkg/system-probe/config/adjust.go`. Sk-tracer disables USM in
//! `pkg/system-probe/config/adjust_npm.go`; discovery conflicts in
//! `pkg/system-probe/config/adjust_discovery.go`. Module knob resolution uses
//! highest-priority configured source among fleet, env, and YAML
//! ([`super::env_bindings`], `pkg/config/model/types.go` precedence). EUD
//! infra-mode defaults for core-agent keys use pre-fleet `infrastructure_mode`
//! because `applyInfrastructureModeOverrides` runs before `MergeFleetPolicy`.
//!
//! **When module enablement changes in Go, update `derived_enabled` below.**

use std::path::Path;

use super::YamlCache;
use super::env_bindings::env_bool_for_config_key;

const SYSPROBE_FLEET: &str = "system-probe.yaml";
const AGENT_FLEET: &str = "datadog.yaml";

/// Keys set to true by `applyInfrastructureModeOverrides` for `end_user_device`.
const EUD_INFRA_MODE_AGENT_KEYS: &[&str] = &[
    "software_inventory.enabled",
    "notable_events.enabled",
    "logon_duration.enabled",
];

/// Returns whether any system-probe module would be enabled at runtime (post-`Adjust`).
pub(super) fn derived_enabled(sysprobe_path: &str, yaml: &mut YamlCache) -> anyhow::Result<bool> {
    let agent = Path::new(sysprobe_path)
        .parent()
        .map(|dir| dir.join("datadog.yaml"))
        .map(|path| path.to_string_lossy().into_owned())
        .unwrap_or_else(|| sysprobe_path.to_owned());

    let mut cfg = Cfg {
        sysprobe: sysprobe_path,
        agent: &agent,
        yaml,
    };

    // config.go:123-131 — values reused below (post-Adjust; USM may be cleared by sk tracer).
    let npm = cfg.npm_enabled()?;
    let usm = cfg.effective_usm_enabled()?;
    let ccm = cfg.sp_get_bool("ccm_network_config.enabled")?;
    let eudm = cfg
        .agent_string("infrastructure_mode")?
        .is_some_and(|m| m == "end_user_device");
    let csm = cfg.sp_get_bool("runtime_security_config.enabled")?;
    let gpu = cfg.sp_get_bool("gpu_monitoring.enabled")?;
    let di = cfg.sp_get_bool("dynamic_instrumentation.enabled")?;
    let discovery_service_map = cfg.effective_discovery_service_map()?;

    // config.go:133-135 — NetworkTracerModule
    let network_tracer = npm
        || usm
        || ccm
        || eudm
        || discovery_service_map
        || (csm && cfg.sp_get_bool("runtime_security_config.network_monitoring.enabled")?);
    if network_tracer {
        return Ok(true);
    }

    // config.go:136-141 — TCP queue length, OOM kill
    if cfg.sp_get_bool("system_probe_config.enable_tcp_queue_length")?
        || cfg.sp_get_bool("system_probe_config.enable_oom_kill")?
    {
        return Ok(true);
    }

    // config.go:142-150 — EventMonitorModule
    // `network_process.enabled` needs NetworkTracerModule too (config.go:146); when that is on we
    // already returned above.
    if csm
        || cfg.sp_get_bool("runtime_security_config.fim_enabled")?
        || cfg.agent_get_bool("sbom.enrichment.usage.enabled")?
        || (usm && cfg.sp_get_bool("service_monitoring_config.enable_event_stream")?)
        || gpu
        || di
    {
        return Ok(true);
    }

    // config.go:151-164 — ComplianceModule
    if (cfg.agent_get_bool("compliance_config.enabled")?
        && cfg.agent_get_bool("compliance_config.run_in_system_probe")?)
        || cfg.sp_get_bool("compliance_config.database_benchmarks.enabled")?
        || (csm && cfg.sp_get_bool("runtime_security_config.compliance_module.enabled")?)
    {
        return Ok(true);
    }

    // config.go:187-188 — DiscoveryModule (schema platform_default: linux true, fargate/other false)
    if cfg.effective_discovery_enabled()? {
        return Ok(true);
    }

    // config.go:165-194 — remaining modules with a single config knob each
    for key in [
        "system_probe_config.process_config.enabled",
        "ebpf_check.enabled",
        "system_probe_config.language_detection.enabled",
        "ping.enabled",
        "traceroute.enabled",
        "privileged_logs.enabled",
        "noisy_neighbor.enabled",
        "windows_crash_detection.enabled",
    ] {
        if cfg.sp_get_bool(key)? {
            return Ok(true);
        }
    }

    // config.go:199-203: LogonDurationModule reads core datadog.yaml, not system-probe.yaml.
    #[cfg(target_os = "macos")]
    if cfg.agent_get_bool("logon_duration.enabled")? {
        return Ok(true);
    }

    // config.go:221 — NotableEventsModule reads core datadog.yaml, not system-probe.yaml.
    #[cfg(target_os = "macos")]
    if cfg.agent_get_bool("notable_events.enabled")? {
        return Ok(true);
    }

    // config.go:210-221 — Windows/macOS modules with their own knob.
    // Injector default-on-when-other-modules-enabled is skipped: it cannot be the
    // first module to turn system-probe on. Auto-enabled Windows crash detection
    // likewise requires network tracer or event monitor, already handled above.
    #[cfg(any(windows, target_os = "macos"))]
    {
        if cfg.agent_get_bool("software_inventory.enabled")?
            || cfg.sp_get_bool("injector.enable_telemetry")?
        {
            return Ok(true);
        }
    }

    Ok(false)
}

struct Cfg<'a> {
    sysprobe: &'a str,
    agent: &'a str,
    yaml: &'a mut YamlCache,
}

impl<'a> Cfg<'a> {
    fn sp_get_bool(&mut self, key: &str) -> anyhow::Result<bool> {
        self.yaml
            .resolve_get_bool(self.sysprobe, key, Some(SYSPROBE_FLEET))
    }

    fn sp_get_bool_default(&mut self, key: &str, default: bool) -> anyhow::Result<bool> {
        self.yaml.resolve_get_bool_with_default(
            self.sysprobe,
            key,
            Some(SYSPROBE_FLEET),
            default,
        )
    }

    fn agent_get_bool(&mut self, key: &str) -> anyhow::Result<bool> {
        if let Some(enabled) = self.agent_get_bool_if_set(key)? {
            return Ok(enabled);
        }
        Ok(self.infra_mode_eud_agent_bool(key))
    }

    fn agent_get_bool_if_set(&mut self, key: &str) -> anyhow::Result<Option<bool>> {
        if let Some(path) = self.yaml.fleet_policy_path(AGENT_FLEET, self.agent)?
            && self.yaml.dotted_key_if_exists(&path, key)?.is_some()
        {
            return Ok(Some(self.yaml.get_bool_at(&path, key)?));
        }
        if let Some(enabled) = env_bool_for_config_key(key) {
            return Ok(Some(enabled));
        }
        if self.yaml.dotted_key_if_exists(self.agent, key)?.is_some() {
            return Ok(Some(self.yaml.get_bool_at(self.agent, key)?));
        }
        Ok(None)
    }

    /// `applyInfrastructureModeOverrides` runs before `MergeFleetPolicy` and is not
    /// re-run after fleet merge, so retained EUD defaults use pre-fleet infra mode.
    fn infra_mode_eud_agent_bool(&mut self, key: &str) -> bool {
        if !EUD_INFRA_MODE_AGENT_KEYS.contains(&key) {
            return false;
        }
        self.yaml
            .resolve_string_pre_fleet(self.agent, "infrastructure_mode")
            .ok()
            .flatten()
            .is_some_and(|mode| mode == "end_user_device")
    }

    fn agent_string(&mut self, key: &str) -> anyhow::Result<Option<String>> {
        self.yaml.resolve_string(self.agent, key, Some(AGENT_FLEET))
    }

    fn sp_is_configured(&mut self, key: &str) -> anyhow::Result<bool> {
        self.yaml
            .is_configured(self.sysprobe, key, Some(SYSPROBE_FLEET))
    }

    /// adjust.go: `system_probe_config.enabled: true` with no NPM/USM block enables NPM.
    fn npm_enabled(&mut self) -> anyhow::Result<bool> {
        if self.sp_get_bool("network_config.enabled")? {
            return Ok(true);
        }
        // Network: IsConfigured; USM: !GetBool. Back-compat runs before adjustNetwork (raw USM).
        if self.sp_get_bool("system_probe_config.enabled")?
            && !self.sp_is_configured("network_config.enabled")?
            && !self.sp_get_bool("service_monitoring_config.enabled")?
        {
            return Ok(true);
        }
        Ok(false)
    }

    fn effective_usm_enabled(&mut self) -> anyhow::Result<bool> {
        // adjust_npm.go:123-135 — sk tracer disables USM when enabled.
        let sk_tracer = self.sp_get_bool("network_config.enable_sk_tracer")?
            && self.sp_get_bool_default("system_probe_config.enable_co_re", true)?
            && self.sp_get_bool_default("network_config.enable_ringbuffers", true)?;
        if sk_tracer {
            return Ok(false);
        }
        self.sp_get_bool("service_monitoring_config.enabled")
    }

    fn effective_discovery_enabled(&mut self) -> anyhow::Result<bool> {
        self.sp_get_bool_default("discovery.enabled", discovery_enabled_platform_default())
    }

    fn effective_discovery_service_map(&mut self) -> anyhow::Result<bool> {
        if !self.sp_get_bool("discovery.service_map.enabled")? {
            return Ok(false);
        }
        // adjust_discovery.go:48-52 — full USM makes discovery redundant (pre-adjustNetwork).
        if self.sp_get_bool("service_monitoring_config.enabled")? {
            return Ok(false);
        }
        // adjust_discovery.go:61-77 — raw sk_tracer / ebpfless (before adjustNetwork).
        if self.sp_get_bool("network_config.enable_sk_tracer")?
            || self.sp_get_bool("network_config.enable_ebpfless")?
        {
            return Ok(false);
        }
        Ok(true)
    }
}

/// Schema `platform_default` for `discovery.enabled`.
fn discovery_enabled_platform_default() -> bool {
    #[cfg(not(target_os = "linux"))]
    {
        false
    }
    #[cfg(target_os = "linux")]
    {
        !is_ecs_fargate()
    }
}

/// Mirrors `IsECSFargate` in `pkg/config/env/environment.go`.
#[cfg(target_os = "linux")]
fn is_ecs_fargate() -> bool {
    std::env::var("ECS_FARGATE").is_ok_and(|v| !v.is_empty())
        || matches!(
            std::env::var("AWS_EXECUTION_ENV").ok().as_deref(),
            Some("AWS_ECS_FARGATE")
        )
}
