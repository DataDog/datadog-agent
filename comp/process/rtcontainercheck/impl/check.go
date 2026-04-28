// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtcontainercheckimpl implements a component to handle realtime Container data collection in the Process Agent.
package rtcontainercheckimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	rtcontainercheck "github.com/DataDog/datadog-agent/comp/process/rtcontainercheck/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

var _ types.CheckComponent = (*check)(nil)

type check struct {
	rtContainerCheck *checks.RTContainerCheck
}

type dependencies struct {
	compdef.In

	Config    config.Component
	Sysconfig sysprobeconfig.Component
	WMmeta    workloadmeta.Component
}

type Provides struct {
	compdef.Out

	Check     types.ProvidesCheck
	Component rtcontainercheck.Component
}

// NewCheck creates a new rtcontainercheck component.
func NewCheck(deps dependencies) Provides {
	c := &check{
		rtContainerCheck: checks.NewRTContainerCheck(deps.Config, deps.Sysconfig, deps.WMmeta),
	}
	return Provides{
		Check: types.ProvidesCheck{
			CheckComponent: c,
		},
		Component: c,
	}
}

func (c *check) Object() checks.Check {
	return c.rtContainerCheck
}
