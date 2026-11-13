// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func adjustSecurity(cfg model.Config) {
	deprecateCustom(cfg, secNS("activity_dump.cgroup_dump_timeout"), secNS("activity_dump.dump_duration"), func(cfg model.Config) interface{} {
		// convert old minutes int value to time.Duration
		return time.Duration(cfg.GetInt(secNS("activity_dump.cgroup_dump_timeout"))) * time.Minute
	})

	deprecateCustom(
		cfg,
		secNS("runtime_security_config.security_profile.anomaly_detection.auto_suppression.enabled"),
		secNS("runtime_security_config.security_profile.auto_suppression.enabled"),
		func(cfg model.Config) interface{} {
			// convert old auto suppression parameter to the new one
			return cfg.GetBool(secNS("runtime_security_config.security_profile.anomaly_detection.auto_suppression.enabled"))
		},
	)

	if cfg.GetBool(secNS("enabled")) {
		// if runtime is enabled then we enable fim as well (except if force disabled)
		if runtime.GOOS != "windows" || !cfg.IsConfigured(secNS("fim_enabled")) {
			cfg.Set(secNS("fim_enabled"), true, model.SourceAgentRuntime)
		}
	} else {
		// if runtime is disabled then we force disable activity dumps and security profiles
		cfg.Set(secNS("activity_dump.enabled"), false, model.SourceAgentRuntime)
		cfg.Set(secNS("security_profile.enabled"), false, model.SourceAgentRuntime)
	}

	// The core agent loads this configuration too, and config streaming hands
	// its core configuration to system-probe, so both read the value set here.
	coreCfg := pkgconfigsetup.Datadog()
	if !coreCfg.IsConfigured(sbomUsageKey) && sbomUsageByDefault(cfg, coreCfg) {
		coreCfg.Set(sbomUsageKey, true, model.SourceAgentRuntime)
	}
	if coreCfg.GetBool(sbomUsageKey) {
		cfg.Set(secNS("event_sampling.open.enabled"), true, model.SourceAgentRuntime)
	}

	// further adjustments done in RuntimeSecurityConfig.sanitize
	// because it requires access to security packages
}

// sbomUsageKey is the setting of the runtime usage enrichment of SBOMs.
const sbomUsageKey = "sbom.enrichment.usage.enabled"

// sbomUsageByDefault reports whether the runtime usage enrichment of SBOMs is
// on while sbom.enrichment.usage.enabled is unset. It is, on Linux, where the
// Agent scans the SBOMs of container images or of the host and the eBPF probe
// of CWS or FIM runs, as that probe records the usage.
func sbomUsageByDefault(cfg, coreCfg model.Reader) bool {
	if runtime.GOOS != "linux" {
		return false
	}

	containers := coreCfg.GetBool("container_image.enabled") && coreCfg.GetBool("sbom.container_image.enabled")
	scans := coreCfg.GetBool("sbom.enabled") && (containers || coreCfg.GetBool("sbom.host.enabled"))
	cws := cfg.GetBool(secNS("enabled")) || cfg.GetBool(secNS("fim_enabled"))
	return scans && cws && !ebpfLess(cfg, coreCfg)
}

// ebpfLess reports whether CWS runs without eBPF: as
// runtime_security_config.ebpfless.enabled says, or, while it is unset, in a
// sidecar, as pkg/security/config.IsEBPFLessModeEnabled decides it later. The
// sidecar test copies fargate.IsSidecar, whose package would bring the ECS
// metadata client and its dependencies into every importer of this one.
func ebpfLess(cfg, coreCfg model.Reader) bool {
	if cfg.IsConfigured(secNS("ebpfless.enabled")) {
		return cfg.GetBool(secNS("ebpfless.enabled"))
	}
	return env.IsFeaturePresent(env.ECSFargate) || env.IsFeaturePresent(env.EKSFargate) || env.IsECSSidecarMode(coreCfg)
}
