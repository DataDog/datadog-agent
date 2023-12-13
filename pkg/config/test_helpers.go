// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var (
	// SetFeatures is alias from env
	SetFeatures = env.SetFeatures
	// SetFeaturesNoCleanup is alias from env
	SetFeaturesNoCleanup = env.SetFeaturesNoCleanup

	// SetupConf generates and returns a new configuration
	SetupConf = pkgconfigsetup.Conf

	// SetupConfFromYAML generates a configuration from the given yaml config
	SetupConfFromYAML = pkgconfigsetup.ConfFromYAML
)

// ResetSystemProbeConfig resets the configuration.
func ResetSystemProbeConfig(t *testing.T) {
	originalConfig := SystemProbe
	t.Cleanup(func() {
		SystemProbe = originalConfig
	})
	SystemProbe = NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	pkgconfigsetup.InitSystemProbeConfig(SystemProbe)
}
