// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectionscheckimpl implements a component to handle Connections data collection in the Process Agent.
package connectionscheckimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/process/connectionscheck"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newCheck))
}

var _ types.CheckComponent = (*check)(nil)

type check struct {
	connectionsCheck *checks.ConnectionsCheck
}

type dependencies struct {
	fx.In

	Sysconfig sysprobeconfig.Component
	Config    config.Component
	WMeta     workloadmeta.Component
}

type result struct {
	fx.Out

	Check     types.ProvidesCheck
	Component connectionscheck.Component
}

func newCheck(deps dependencies) result {
	c := &check{
		connectionsCheck: checks.NewConnectionsCheck(deps.Config, deps.Sysconfig, deps.Sysconfig.SysProbeObject(), deps.WMeta),
	}
	return result{
		Check: types.ProvidesCheck{
			CheckComponent: c,
		},
		Component: c,
	}
}

func (c *check) Object() checks.Check {
	return c.connectionsCheck
}
