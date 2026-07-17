// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Derived `system_probe_config.enabled` for process-manager config gates.
//!
//! # Keep in sync with Go
//!
//! Mirrors `load()` in `pkg/system-probe/config/config.go` and the NPM back-compat
//! rule in `pkg/system-probe/config/adjust.go`. Module knob resolution uses fleet →
//! env → YAML ([`super::env_bindings`], `pkg/config/model/types.go` precedence).
//!
//! **When module enablement changes in Go, update `derived_enabled` below.**

use std::path::Path;

use super::YamlCache;

const SYSPROBE_FLEET: &str = "system-probe.yaml";
const AGENT_FLEET: &str = "datadog.yaml";

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

    // config.go:123-131 — values reused below.
    let npm = cfg.npm_enabled()?;
    let usm = cfg.sp_bool("service_monitoring_config.enabled")?;
    let ccm = cfg.sp_bool("ccm_network_config.enabled")?;
    let eudm = cfg
        .agent_string("infrastructure_mode")?
        .is_some_and(|m| m == "end_user_device");
    let csm = cfg.sp_bool("runtime_security_config.enabled")?;
    let gpu = cfg.sp_bool("gpu_monitoring.enabled")?;
    let di = cfg.sp_bool("dynamic_instrumentation.enabled")?;
    let discovery_service_map = cfg.sp_bool("discovery.service_map.enabled")?;

    // config.go:133-135 — NetworkTracerModule
    let network_tracer = npm
        || usm
        || ccm
        || eudm
        || discovery_service_map
        || (csm && cfg.sp_bool("runtime_security_config.network_monitoring.enabled")?);
    if network_tracer {
        return Ok(true);
    }

    // config.go:136-141 — TCP queue length, OOM kill
    if cfg.sp_bool("system_probe_config.enable_tcp_queue_length")?
        || cfg.sp_bool("system_probe_config.enable_oom_kill")?
    {
        return Ok(true);
    }

    // config.go:142-150 — EventMonitorModule
    // `network_process.enabled` needs NetworkTracerModule too (config.go:146); when that is on we
    // already returned above.
    if csm
        || cfg.sp_bool("runtime_security_config.fim_enabled")?
        || cfg.agent_bool("sbom.enrichment.usage.enabled")?
        || (usm && cfg.sp_bool("service_monitoring_config.enable_event_stream")?)
        || gpu
        || di
    {
        return Ok(true);
    }

    // config.go:151-164 — ComplianceModule
    if (cfg.agent_bool("compliance_config.enabled")?
        && cfg.agent_bool("compliance_config.run_in_system_probe")?)
        || cfg.sp_bool("compliance_config.database_benchmarks.enabled")?
        || (csm && cfg.sp_bool("runtime_security_config.compliance_module.enabled")?)
    {
        return Ok(true);
    }

    // config.go:165-194 — remaining modules with a single config knob each
    for key in [
        "system_probe_config.process_config.enabled",
        "ebpf_check.enabled",
        "system_probe_config.language_detection.enabled",
        "ping.enabled",
        "traceroute.enabled",
        "discovery.enabled",
        "privileged_logs.enabled",
        "noisy_neighbor.enabled",
        "windows_crash_detection.enabled",
    ] {
        if cfg.sp_bool(key)? {
            return Ok(true);
        }
    }

    #[cfg(target_os = "macos")]
    if cfg.sp_bool("logon_duration.enabled")? {
        return Ok(true);
    }

    // config.go:210-221 — Windows/macOS modules with their own knob.
    // Injector default-on-when-other-modules-enabled is skipped: it cannot be the
    // first module to turn system-probe on. Auto-enabled Windows crash detection
    // likewise requires network tracer or event monitor, already handled above.
    #[cfg(any(windows, target_os = "macos"))]
    {
        if cfg.agent_bool("software_inventory.enabled")?
            || cfg.sp_bool("injector.enable_telemetry")?
        {
            return Ok(true);
        }
    }

    Ok(false)
}

/// Resolved config values from system-probe.yaml / datadog.yaml (+ fleet when set).
struct Cfg<'a> {
    sysprobe: &'a str,
    agent: &'a str,
    yaml: &'a mut YamlCache,
}

impl<'a> Cfg<'a> {
    fn sp_bool(&mut self, key: &str) -> anyhow::Result<bool> {
        self.yaml
            .resolve_bool(self.sysprobe, key, Some(SYSPROBE_FLEET))
    }

    fn agent_bool(&mut self, key: &str) -> anyhow::Result<bool> {
        self.yaml.resolve_bool(self.agent, key, Some(AGENT_FLEET))
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
        if self.sp_bool("network_config.enabled")? {
            return Ok(true);
        }
        // Network: Go uses IsConfigured; USM: Go uses !GetBool (explicit `false` still allows back-compat).
        // Keep in sync with `adjust.go` (`!cfg.IsConfigured(netNS("enabled"))`).
        if self.sp_bool("system_probe_config.enabled")?
            && !self.sp_is_configured("network_config.enabled")?
            && !self.sp_bool("service_monitoring_config.enabled")?
        {
            return Ok(true);
        }
        Ok(false)
    }
}
