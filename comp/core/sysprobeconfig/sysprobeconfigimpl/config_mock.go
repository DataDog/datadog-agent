// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sysprobeconfigimpl

import (
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

func newMock(deps dependencies, t testing.TB) sysprobeconfig.Component {
	old := config.SystemProbe
	config.SystemProbe = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		warnings: &config.Warnings{},
	}

	// call InitSystemProbeConfig to set defaults.
	config.InitSystemProbeConfig(config.SystemProbe)

	// Viper's `GetXxx` methods read environment variables at the time they are
	// called, if those names were passed explicitly to BindEnv*(), so we must
	// also strip all `DD_` environment variables for the duration of the test.
	oldEnv := os.Environ()
	for _, kv := range oldEnv {
		if strings.HasPrefix(kv, "DD_") {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Unsetenv(kvslice[0])
		}
	}
	t.Cleanup(func() {
		for _, kv := range oldEnv {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Setenv(kvslice[0], kvslice[1])
		}
	})

	// swap the existing config back at the end of the test.
	t.Cleanup(func() { config.SystemProbe = old })

	syscfg, err := setupConfig(deps)
	if err != nil {
		t.Fatalf("sysprobe config create: %s", err)
	}
	c.syscfg = syscfg
	return c
}
