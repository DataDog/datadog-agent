// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Derived `system_probe_config.enabled` for process-manager config gates.
//!
//! # Keep in sync with Go
//!
//! Mirrors `load()` in `pkg/system-probe/config/config.go`, which sets
//! `system_probe_config.enabled` to whether any module ended up enabled, plus the
//! adjustments in `pkg/system-probe/config/adjust.go` (NPM back-compat),
//! `adjust_npm.go` (the sk tracer clears USM) and `adjust_discovery.go` (discovery
//! service map conflicts).
//!
//! **When module enablement changes in Go, update [`derived_enabled`].**
//!
//! Module knobs come from the highest-priority configured source among fleet policy,
//! env, and YAML. A few modules read the core `datadog.yaml` instead of
//! `system-probe.yaml`; those go through [`Cfg::agent_bool`], which also applies the
//! end-user-device defaults from `applyInfrastructureModeOverrides`.

use super::{AGENT_POLICY, HostOs, SYSPROBE_POLICY, YamlCache};

/// Core Agent keys that `applyInfrastructureModeOverrides` turns on for `end_user_device`.
const EUD_INFRA_MODE_AGENT_KEYS: &[&str] = &[
    "software_inventory.enabled",
    "notable_events.enabled",
    "logon_duration.enabled",
];

/// Modules gated on a single `system-probe.yaml` knob.
const SINGLE_KNOB_MODULES: &[&str] = &[
    "system_probe_config.process_config.enabled",
    "ebpf_check.enabled",
    "system_probe_config.language_detection.enabled",
    "ping.enabled",
    // config.go: traceroute also auto-enables for CNM dynamic tests, but only when NPM
    // is on, and NPM alone already enables system-probe above.
    "traceroute.enabled",
    "privileged_logs.enabled",
    "noisy_neighbor.enabled",
    "windows_crash_detection.enabled",
];

/// Whether any system-probe module would be enabled at runtime (post-`Adjust`).
pub(super) fn derived_enabled(sysprobe_path: &str, yaml: &mut YamlCache, os: HostOs) -> bool {
    let agent =
        super::path_to_string(std::path::Path::new(sysprobe_path).with_file_name(AGENT_POLICY));
    let mut cfg = Cfg {
        sysprobe: sysprobe_path,
        agent: &agent,
        yaml,
        os,
    };

    // Values reused across module checks, matching the locals in config.go.
    let npm = cfg.npm_enabled();
    let usm = cfg.usm_enabled();
    let csm = cfg.sysprobe_bool("runtime_security_config.enabled");
    let gpu = cfg.sysprobe_bool("gpu_monitoring.enabled");
    let di = cfg.sysprobe_bool("dynamic_instrumentation.enabled");

    // NetworkTracerModule
    if npm
        || usm
        || cfg.sysprobe_bool("ccm_network_config.enabled")
        // config.go reads infrastructure_mode at runtime, so a fleet policy that changes
        // the mode does suppress this term, unlike the defaults it wrote at load time.
        || cfg.yaml.end_user_device_effective(&agent)
        || cfg.discovery_service_map_enabled()
        || (csm && cfg.sysprobe_bool("runtime_security_config.network_monitoring.enabled"))
    {
        return true;
    }

    // TCPQueueLengthTracerModule, OOMKillProbeModule
    if cfg.sysprobe_bool("system_probe_config.enable_tcp_queue_length")
        || cfg.sysprobe_bool("system_probe_config.enable_oom_kill")
    {
        return true;
    }

    // EventMonitorModule, plus GPUMonitoringModule and DynamicInstrumentationModule,
    // which share the same knobs. `event_monitoring_config.network_process.enabled` also
    // needs the network tracer, which already returned above.
    if csm
        || cfg.sysprobe_bool("runtime_security_config.fim_enabled")
        || cfg.agent_bool("sbom.enrichment.usage.enabled")
        || (usm && cfg.sysprobe_bool("service_monitoring_config.enable_event_stream"))
        || gpu
        || di
    {
        return true;
    }

    // ComplianceModule
    if (cfg.agent_bool("compliance_config.enabled")
        && cfg.agent_bool("compliance_config.run_in_system_probe"))
        || cfg.sysprobe_bool("compliance_config.database_benchmarks.enabled")
        || (csm && cfg.sysprobe_bool("runtime_security_config.compliance_module.enabled"))
    {
        return true;
    }

    // DiscoveryModule
    if cfg.discovery_enabled() {
        return true;
    }

    if SINGLE_KNOB_MODULES.iter().any(|key| cfg.sysprobe_bool(key)) {
        return true;
    }

    // LogonDurationModule and NotableEventsModule read the core datadog.yaml.
    if os == HostOs::MacOs
        && (cfg.agent_bool("logon_duration.enabled") || cfg.agent_bool("notable_events.enabled"))
    {
        return true;
    }

    // SoftwareInventoryModule and InjectorModule. The injector's
    // default-on-when-other-modules-are-enabled rule is skipped: it can never be the
    // first module to turn system-probe on. Auto-enabled Windows crash detection
    // likewise requires the network tracer or event monitor, handled above.
    if matches!(os, HostOs::Windows | HostOs::MacOs)
        && (cfg.agent_bool("software_inventory.enabled")
            || cfg.sysprobe_bool("injector.enable_telemetry"))
    {
        return true;
    }

    false
}

struct Cfg<'a> {
    sysprobe: &'a str,
    agent: &'a str,
    yaml: &'a mut YamlCache,
    os: HostOs,
}

impl Cfg<'_> {
    fn sysprobe_bool(&mut self, key: &str) -> bool {
        self.sysprobe_bool_or(key, false)
    }

    fn sysprobe_bool_or(&mut self, key: &str, default: bool) -> bool {
        self.yaml
            .resolve_bool_or(self.sysprobe, key, SYSPROBE_POLICY, default)
    }

    /// A core `datadog.yaml` key, including the end-user-device defaults that
    /// `applyInfrastructureModeOverrides` applies when no source sets the key.
    fn agent_bool(&mut self, key: &str) -> bool {
        if let Some(enabled) = self.yaml.resolve_bool(self.agent, key, AGENT_POLICY) {
            return enabled;
        }
        EUD_INFRA_MODE_AGENT_KEYS.contains(&key) && self.yaml.end_user_device_at_load(self.agent)
    }

    fn sysprobe_is_configured(&mut self, key: &str) -> bool {
        self.yaml.is_configured(self.sysprobe, key, SYSPROBE_POLICY)
    }

    /// adjust.go: `system_probe_config.enabled: true` with no NPM or USM setting enables NPM.
    fn npm_enabled(&mut self) -> bool {
        if self.sysprobe_bool("network_config.enabled") {
            return true;
        }
        // Network uses IsConfigured, USM uses GetBool. Back-compat runs before
        // adjustNetwork, so USM is read raw.
        self.sysprobe_bool("system_probe_config.enabled")
            && !self.sysprobe_is_configured("network_config.enabled")
            && !self.sysprobe_bool("service_monitoring_config.enabled")
    }

    /// adjust_npm.go: the sk tracer disables USM when it is active.
    fn usm_enabled(&mut self) -> bool {
        let sk_tracer = self.sysprobe_bool("network_config.enable_sk_tracer")
            && self.sysprobe_bool_or("system_probe_config.enable_co_re", true)
            && self.sysprobe_bool_or("network_config.enable_ringbuffers", true);
        !sk_tracer && self.sysprobe_bool("service_monitoring_config.enabled")
    }

    fn discovery_enabled(&mut self) -> bool {
        self.sysprobe_bool_or("discovery.enabled", self.discovery_platform_default())
    }

    /// adjust_discovery.go: full USM makes the service map redundant, and neither the sk
    /// tracer nor ebpfless supports it. All three are read before adjustNetwork.
    fn discovery_service_map_enabled(&mut self) -> bool {
        self.sysprobe_bool("discovery.service_map.enabled")
            && !self.sysprobe_bool("service_monitoring_config.enabled")
            && !self.sysprobe_bool("network_config.enable_sk_tracer")
            && !self.sysprobe_bool("network_config.enable_ebpfless")
    }

    /// Schema `platform_default` for `discovery.enabled`.
    fn discovery_platform_default(&self) -> bool {
        self.os == HostOs::Linux && !is_ecs_fargate()
    }
}

/// Mirrors `IsECSFargate` in `pkg/config/env/environment.go`.
fn is_ecs_fargate() -> bool {
    std::env::var("ECS_FARGATE").is_ok_and(|value| !value.is_empty())
        || matches!(
            std::env::var("AWS_EXECUTION_ENV").ok().as_deref(),
            Some("AWS_ECS_FARGATE")
        )
}
