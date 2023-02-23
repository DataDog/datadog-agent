// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectionscheck

import (
	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

var _ types.CheckComponent = (*check)(nil)

type check struct {
	connectionsCheck *checks.ConnectionsCheck
}

type dependencies struct {
	fx.In

	sysconfig *sysconfig.Config
}

func newCheck(deps dependencies) types.ProvidesCheck {
	return types.ProvidesCheck{
		CheckComponent: &check{
			connectionsCheck: checks.NewConnectionsCheck(deps.sysconfig),
		},
	}
}

func (c *check) Object() checks.Check {
	return c.connectionsCheck
}
