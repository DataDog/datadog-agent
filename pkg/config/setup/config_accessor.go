// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !test

package setup

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

// Datadog returns the current agent configuration
func Datadog() pkgconfigmodel.Config {
	return datadog
}

// SystemProbe returns the current SystemProbe configuration
func SystemProbe() pkgconfigmodel.Config {
	return systemProbe
}

// GlobalConfigBuilder returns a builder appropriate for initializing
// the config. It should not be used in most places, except for code
// that builds the config from scratch.
func GlobalConfigBuilder() pkgconfigmodel.BuildableConfig {
	// NOTE: This is guaranteed safe because `create.New` returns this type
	return datadog.(pkgconfigmodel.BuildableConfig)
}

// GlobalSystemProbeConfigBuilder returns a builder for the system probe config
func GlobalSystemProbeConfigBuilder() pkgconfigmodel.BuildableConfig {
	return systemProbe.(pkgconfigmodel.BuildableConfig)
}
