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
