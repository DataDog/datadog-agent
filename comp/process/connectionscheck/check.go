// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectionscheck

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

var _ types.CheckComponent = (*check)(nil)

type check struct {
	connectionsCheck *checks.ConnectionsCheck
}

type dependencies struct {
	fx.In

	Sysconfig sysprobeconfig.Component
}

type result struct {
	fx.Out

	Check     types.ProvidesCheck
	Component Component
}

func newCheck(deps dependencies) result {
	c := &check{
		connectionsCheck: checks.NewConnectionsCheck(deps.Sysconfig.Object()),
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
