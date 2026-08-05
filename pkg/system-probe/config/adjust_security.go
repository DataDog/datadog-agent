// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"
	"time"

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

	// The host-wide capture window is built on CWS and the V2 security profile manager, so the
	// single host_dump switch has to bring both up. This has to happen here, on the raw
	// configuration and before the checks below: module enablement (event_monitor) reads
	// runtime_security_config.enabled directly, and the branch below would otherwise force
	// security_profile.enabled back off.
	if cfg.GetBool(secNS("security_profile.v2.host_dump.enabled")) {
		cfg.Set(secNS("enabled"), true, model.SourceAgentRuntime)
		cfg.Set(secNS("security_profile.enabled"), true, model.SourceAgentRuntime)
		cfg.Set(secNS("security_profile.v2.enabled"), true, model.SourceAgentRuntime)
	}

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

	if pkgconfigsetup.Datadog().GetBool("sbom.enrichment.usage.enabled") {
		cfg.Set(secNS("event_sampling.open.enabled"), true, model.SourceAgentRuntime)
	}

	// further adjustments done in RuntimeSecurityConfig.sanitize
	// because it requires access to security packages
}
