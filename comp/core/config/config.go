// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"
	"testing"

	"go.uber.org/fx"

	secconfig "github.com/DataDog/datadog-agent/cmd/security-agent/config"
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
		if e != nil && deps.Params.configInvalidOK {
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
		if err := secconfig.Merge(deps.Params.securityAgentConfigFilePaths); err != nil {
			returnErrFct(err)
		}
	}

	return &cfg{Config: config.Datadog, warnings: warnings}, nil
}

func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func newMock(deps dependencies, t testing.TB) Component {
	old := config.Datadog
	config.Datadog = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		warnings: &config.Warnings{},
	}

	// call InitConfig to set defaults.
	config.InitConfig(config.Datadog)

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
	t.Cleanup(func() { config.Datadog = old })

	return c
}
