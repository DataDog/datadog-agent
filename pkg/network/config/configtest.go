// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"testing"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// ResetConfig resets the configuration.
func ResetConfig(t *testing.T) {
	originalConfig := aconfig.SystemProbe
	t.Cleanup(func() {
		aconfig.SystemProbe = originalConfig
	})
	aconfig.SystemProbe = aconfig.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	aconfig.InitSystemProbeConfig(aconfig.SystemProbe)
}
