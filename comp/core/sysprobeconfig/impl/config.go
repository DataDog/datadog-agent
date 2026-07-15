// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sysprobeconfigimpl implements a component to handle system-probe configuration.  This
// component temporarily wraps pkg/config.
package sysprobeconfigimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	sysprobeconfigdef "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.
	model.Config

	syscfg *sysconfigtypes.Config

	// warnings are the warnings generated during setup
	warnings *model.Warnings
}

// Requires defines the dependencies of the sysprobeconfig component.
type Requires struct {
	compdef.In

	Params Params
	// Enforce loading order between core agent config and system-probe config
	CoreConfig config.Component
}

// Provides defines the outputs of the sysprobeconfig component.
type Provides struct {
	compdef.Out

	Comp sysprobeconfigdef.Component
}

// NewComponent creates a new sysprobeconfig component.
func NewComponent(deps Requires) (Provides, error) {
	c, err := newConfig(deps)
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: c}, nil
}

func setupConfig(sysProbeConfFilePath string, fleetPoliciesDirPath string) (*sysconfigtypes.Config, error) {
	return sysconfig.New(sysProbeConfFilePath, fleetPoliciesDirPath)
}

func newConfig(deps Requires) (sysprobeconfigdef.Component, error) {
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
