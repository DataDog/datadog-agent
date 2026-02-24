// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sysprobeconfigimpl implements a component to handle system-probe configuration.  This
// component temporarily wraps pkg/config.
package sysprobeconfigimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newConfig),
		fxutil.ProvideOptional[sysprobeconfig.Component](),
	)
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	model.Config

	syscfg *sysconfigtypes.Config

	// warnings are the warnings generated during setup
	warnings *model.Warnings
}

type dependencies struct {
	fx.In

	Params Params
	// Enforce loading order between core agent config and system-probe config
	CoreConfig config.Component
}

func setupConfig(sysProbeConfFilePath string, fleetPoliciesDirPath string) (*sysconfigtypes.Config, error) {
	return sysconfig.New(sysProbeConfFilePath, fleetPoliciesDirPath)
}

func newConfig(deps dependencies) (sysprobeconfig.Component, error) {
	syscfg, err := setupConfig(deps.Params.sysProbeConfFilePath, deps.Params.fleetPoliciesDirPath)
	if err != nil {
		return nil, err
	}

	return &cfg{Config: pkgconfigsetup.SystemProbe(), syscfg: syscfg}, nil
}

func (c *cfg) Warnings() *model.Warnings {
	return c.warnings
}

func (c *cfg) Object() model.Reader {
	return c
}

func (c *cfg) SysProbeObject() *sysconfigtypes.Config {
	return c.syscfg
}
