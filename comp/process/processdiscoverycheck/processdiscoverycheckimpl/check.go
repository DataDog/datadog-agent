// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processdiscoverycheckimpl implements a component to handle Process Discovery data collection in the Process Agent for customers who do not pay for live processes.
package processdiscoverycheckimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/process/processdiscoverycheck"
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
	processDiscoveryCheck *checks.ProcessDiscoveryCheck
}

type dependencies struct {
	fx.In

	Config config.Component
}

type result struct {
	fx.Out

	Check     types.ProvidesCheck
	Component processdiscoverycheck.Component
}

func newCheck(deps dependencies) result {
checks.

	c := &check{
		processDiscoveryCheck: checks.NewProcessDiscoveryCheck(deps.Config),
	}
	return result{
		Check: types.ProvidesCheck{
			CheckComponent: c,
		},
		Component: c,
	}
}

func (c *check) Object() checks.Check {
	return c.processDiscoveryCheck
}
