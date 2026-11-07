// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func adjustSecurity(cfg model.Config) {
	if cfg.GetBool(secNS("enabled")) {
		// if runtime is enabled then we enable fim as well (except if force disabled)
		if runtime.GOOS != "windows" || !cfg.IsConfigured(secNS("fim_enabled")) {
			cfg.Set(secNS("fim_enabled"), true, model.SourceAgentRuntime)
		}
	} else {
		// if runtime is disabled then we force disable security profiles
		cfg.Set(secNS("security_profile.v2.enabled"), false, model.SourceAgentRuntime)
	}

	if pkgconfigsetup.Datadog().GetBool("sbom.enrichment.usage.enabled") {
		cfg.Set(secNS("event_sampling.open.enabled"), true, model.SourceAgentRuntime)
	}

	// further adjustments done in RuntimeSecurityConfig.sanitize
	// because it requires access to security packages
}
