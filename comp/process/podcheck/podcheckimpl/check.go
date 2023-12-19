// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package podcheckimpl implements a component to handle Kubernetes data collection in the Process Agent.
package podcheckimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/podcheck"
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
	podCheck *checks.PodCheck
}

type result struct {
	fx.Out

	Check     types.ProvidesCheck
	Component podcheck.Component
}

func newCheck() result {
	c := &check{
		podCheck: checks.NewPodCheck(),
	}
	return result{
		Check: types.ProvidesCheck{
			CheckComponent: c,
		},
		Component: c,
	}
}

func (c *check) Object() checks.Check {
	return c.podCheck
}
