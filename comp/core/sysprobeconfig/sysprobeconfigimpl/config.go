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
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newConfig),
		fx.Provide(func(syscfg sysprobeconfig.Component) optional.Option[sysprobeconfig.Component] {
			return optional.NewOption[sysprobeconfig.Component](syscfg)
		}),
	)
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	config.Config

	syscfg *sysconfigtypes.Config

	// warnings are the warnings generated during setup
	warnings *config.Warnings
}

// sysprobeconfigDependencies is an interface that mimics the fx-oriented dependencies struct (This is copied from the main agent configuration.)
// The goal of this interface is to be able to call setupConfig with either 'dependencies' or 'mockDependencies'.
// TODO: (components) investigate whether this interface is worth keeping, otherwise delete it and just use dependencies
type sysprobeconfigDependencies interface {
	getParams() *Params
}

type dependencies struct {
	fx.In

	Params Params
}

func (d dependencies) getParams() *Params {
	return &d.Params
}

func setupConfig(deps sysprobeconfigDependencies) (*sysconfigtypes.Config, error) {
	return sysconfig.New(deps.getParams().sysProbeConfFilePath)
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

func (c *cfg) SysProbeObject() *sysconfigtypes.Config {
	return c.syscfg
}
