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
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type mockDependencies struct {
	fx.In

	Params MockParams
}

func (m mockDependencies) getParams() *Params {
	p := m.Params.Params
	return &p
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fxutil.ProvideOptional[sysprobeconfig.Component](),
		fx.Supply(MockParams{}))
}

func newMock(deps mockDependencies, t testing.TB) sysprobeconfig.Component {
	old := setup.SystemProbe()
	setup.SetSystemProbe(model.NewConfig("mock", "XXXX", strings.NewReplacer()))
	c := &cfg{
		warnings: &model.Warnings{},
		Config:   setup.SystemProbe(),
	}

	// call InitSystemProbeConfig to set defaults.
	setup.InitSystemProbeConfig(setup.SystemProbe())

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

	// Overrides are explicit and will take precedence over any other
	// setting
	for k, v := range deps.Params.Overrides {
		setup.SystemProbe().SetWithoutSource(k, v)
	}

	// swap the existing config back at the end of the test.
	t.Cleanup(func() { setup.SetSystemProbe(old) })

	syscfg, err := setupConfig(deps)
	if err != nil {
		t.Fatalf("sysprobe config create: %s", err)
	}
	c.syscfg = syscfg
	return c
}
