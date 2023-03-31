// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	config.Config

	// warnings are the warnings generated during setup
	warnings *config.Warnings
}

type dependencies struct {
	fx.In

	Params Params
}

func newConfig(deps dependencies) (Component, error) {
	warnings, err := setupConfig(deps)
	returnErrFct := func(e error) (Component, error) {
		if e != nil && deps.Params.ignoreErrors {
			if warnings == nil {
				warnings = &config.Warnings{}
			}
			warnings.Err = e
			e = nil
		}
		return &cfg{Config: config.Datadog, warnings: warnings}, e
	}

	if err != nil {
		return returnErrFct(err)
	}

	if deps.Params.configLoadSecurityAgent {
		if err := config.Merge(deps.Params.securityAgentConfigFilePaths); err != nil {
			return returnErrFct(err)
		}
	}

	// Overrides are explicit and will take precedence over any other
	// setting: used in testing
	for k, v := range deps.Params.overrides {
		config.Datadog.Set(k, v)
	}

	return &cfg{Config: config.Datadog, warnings: warnings}, nil
}

func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func (c *cfg) Object() config.ConfigReader {
	return c.Config
}

// NewMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func newMock(deps dependencies, t testing.TB) (Component, error) {
	old := config.Datadog
	config.Datadog = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		Config:   config.Datadog,
		warnings: &config.Warnings{},
	}

	// call InitConfig to set defaults.
	config.InitConfig(config.Datadog)

	warnings, _ := setupConfig(deps)
	c.warnings = warnings

	// Overrides are explicit and will take precedence over any other
	// setting
	for k, v := range deps.Params.overrides {
		config.Datadog.Set(k, v)
	}

	// swap the existing config back at the end of the test.
	t.Cleanup(func() { config.Datadog = old })

	return c, nil
}
