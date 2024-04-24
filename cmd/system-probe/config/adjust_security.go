// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func adjustSecurity(cfg config.Config) {
	deprecateCustom(cfg, secNS("activity_dump.cgroup_dump_timeout"), secNS("activity_dump.dump_duration"), func(cfg config.Config) interface{} {
		// convert old minutes int value to time.Duration
		return time.Duration(cfg.GetInt(secNS("activity_dump.cgroup_dump_timeout"))) * time.Minute
	})

	deprecateCustom(
		cfg,
		secNS("runtime_security_config.security_profile.anomaly_detection.auto_suppression.enabled"),
		secNS("runtime_security_config.security_profile.auto_suppression.enabled"),
		func(cfg config.Config) interface{} {
			// convert old auto suppression parameter to the new one
			return cfg.GetBool(secNS("runtime_security_config.security_profile.anomaly_detection.auto_suppression.enabled"))
		},
	)

	if cfg.GetBool(secNS("enabled")) {
		// if runtime is enabled then we enable fim as well (except if force disabled)
		if !cfg.IsSet(secNS("fim_enabled")) {
			cfg.Set(secNS("fim_enabled"), true, model.SourceAgentRuntime)
		}
	} else {
		// if runtime is disabled then we force disable activity dumps and security profiles
		cfg.Set(secNS("activity_dump.enabled"), false, model.SourceAgentRuntime)
		cfg.Set(secNS("security_profile.enabled"), false, model.SourceAgentRuntime)
	}

	// further adjustments done in RuntimeSecurityConfig.sanitize
	// because it requires access to security packages
}
