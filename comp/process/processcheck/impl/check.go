// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processcheckimpl implements a component to handle Process data collection in the Process Agent.
package processcheckimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpusubscriber "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
	processcheck "github.com/DataDog/datadog-agent/comp/process/processcheck/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

var _ types.CheckComponent = (*check)(nil)

type check struct {
	processCheck *checks.ProcessCheck
}

type dependencies struct {
	compdef.In

	Config        config.Component
	Sysconfig     sysprobeconfig.Component
	WMmeta        workloadmeta.Component
	GpuSubscriber gpusubscriber.Component
	Statsd        statsd.ClientInterface
	IPC           ipc.Component
	Tagger        taggerdef.Component
}

type Provides struct {
	compdef.Out

	Check     types.ProvidesCheck
	Component processcheck.Component
}

// NewCheck creates a new processcheck component.
func NewCheck(deps dependencies) Provides {
	c := &check{
		processCheck: checks.NewProcessCheck(deps.Config, deps.Sysconfig, deps.WMmeta, deps.GpuSubscriber, deps.Statsd, deps.IPC.GetTLSServerConfig(), deps.Tagger),
	}
	return Provides{
		Check: types.ProvidesCheck{
			CheckComponent: c,
		},
		Component: c,
	}
}

func (c *check) Object() checks.Check {
	return c.processCheck
}
