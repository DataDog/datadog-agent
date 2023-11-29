// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Datadog is the global configuration object
var (
	Datadog     = pkgconfigsetup.Datadog
	SystemProbe = pkgconfigsetup.SystemProbe
)

func init() {
	// Configuration defaults
	pkgconfigsetup.InitConfig(Datadog)
	pkgconfigsetup.InitSystemProbeConfig(SystemProbe)
}
