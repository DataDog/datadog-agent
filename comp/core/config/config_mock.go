// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

func (m mockDependencies) getSecretResolver() secrets.Component {
	return nil
}

// newMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func newMock(deps mockDependencies, t testing.TB) (Component, error) {
	backupConfig := pkgconfigmodel.NewConfig("", "", strings.NewReplacer())
	backupConfig.CopyConfig(pkgconfigsetup.Datadog)

	pkgconfigsetup.Datadog.CopyConfig(pkgconfigmodel.NewConfig("mock", "XXXX", strings.NewReplacer()))

	env.SetFeatures(t, deps.Params.Features...)

	// call InitConfig to set defaults.
	pkgconfigsetup.InitConfig(pkgconfigsetup.Datadog)
	c := &cfg{
		Config: pkgconfigsetup.Datadog,
	}

	if !deps.Params.SetupConfig {
		if deps.Params.ConfFilePath != "" {
			pkgconfigsetup.Datadog.SetConfigType("yaml")
			err := pkgconfigsetup.Datadog.ReadConfig(strings.NewReader(deps.Params.ConfFilePath))
			if err != nil {
				// The YAML was invalid, fail initialization of the mock config.
				return nil, err
			}
		}
	} else {
		warnings, _ := setupConfig(pkgconfigsetup.Datadog, deps)
		c.warnings = warnings
	}

	// Overrides are explicit and will take precedence over any other
	// setting
	for k, v := range deps.Params.Overrides {
		pkgconfigsetup.Datadog.SetWithoutSource(k, v)
	}

	// swap the existing config back at the end of the test.
	t.Cleanup(func() { pkgconfigsetup.Datadog.CopyConfig(backupConfig) })

	return c, nil
}
