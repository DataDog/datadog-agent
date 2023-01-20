// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobeconfig

import (
	"os"
	"strings"
	"testing"

	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	config.Config

	syscfg *sysconfig.Config

	// warnings are the warnings generated during setup
	warnings *config.Warnings
}

type dependencies struct {
	fx.In

	Params Params
}

func newConfig(deps dependencies) (Component, error) {
	syscfg, err := setupConfig(deps)
	if err != nil {
		return nil, err
	}

	return &cfg{Config: config.SystemProbe, syscfg: syscfg}, nil
}

func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func (c *cfg) Object() *sysconfig.Config {
	return c.syscfg
}

func newMock(deps dependencies, t testing.TB) Component {
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

	return c
}
