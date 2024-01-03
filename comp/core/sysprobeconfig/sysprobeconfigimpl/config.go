// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sysprobeconfigimpl implements a component to handle system-probe configuration.  This
// component temporarily wraps pkg/config.
package sysprobeconfigimpl

import (
	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newConfig))
}

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

func setupConfig(deps dependencies) (*sysconfig.Config, error) {
	return sysconfig.New(deps.Params.sysProbeConfFilePath)
}

func newConfig(deps dependencies) (sysprobeconfig.Component, error) {
	syscfg, err := setupConfig(deps)
	if err != nil {
		return nil, err
	}

	return &cfg{Config: config.SystemProbe, syscfg: syscfg}, nil
}

func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func (c *cfg) Object() config.Reader {
	return c
}

func (c *cfg) SysProbeObject() *sysconfig.Config {
	return c.syscfg
}
