// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

const (
	evNS = "event_monitoring_config"
)

// Config defines the config
type Config struct {
	EnvVarsResolutionEnabled bool
}

// NewConfig creates a config for the event monitoring module
func NewConfig() *Config {
	return &Config{
		// options
		EnvVarsResolutionEnabled: pkgconfigsetup.SystemProbe().GetBool(sysconfig.FullKeyPath(evNS, "env_vars_resolution.enabled")),
	}
}
