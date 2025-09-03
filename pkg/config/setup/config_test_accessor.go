// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

// Datadog returns the current agent configuration
func Datadog() pkgconfigmodel.Config {
	datadogMutex.RLock()
	defer datadogMutex.RUnlock()
	return datadog
}

// SystemProbe returns the current SystemProbe configuration
func SystemProbe() pkgconfigmodel.Config {
	systemProbeMutex.RLock()
	defer systemProbeMutex.RUnlock()
	return systemProbe
}

// GlobalConfigBuilder returns a builder appropriate for initializing
// the config. It should not be used in most places, except for code
// that builds the config from scratch.
func GlobalConfigBuilder() pkgconfigmodel.BuildableConfig {
	datadogMutex.RLock()
	defer datadogMutex.RUnlock()
	return datadog.(pkgconfigmodel.BuildableConfig)
}

// GlobalSystemProbeConfigBuilder returns a builder for the system probe config
func GlobalSystemProbeConfigBuilder() pkgconfigmodel.BuildableConfig {
	systemProbeMutex.RLock()
	defer systemProbeMutex.RUnlock()
	return systemProbe.(pkgconfigmodel.BuildableConfig)
}
