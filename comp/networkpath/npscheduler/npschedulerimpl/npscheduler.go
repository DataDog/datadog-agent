// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npschedulerimpl implements the scheduler for network path
package npschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	EpForwarder eventplatform.Component
}

type provides struct {
	fx.Out

	Comp npscheduler.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newNpScheduler),
	)
}

func newNpScheduler(deps dependencies) provides {
	// Component initialization
	return provides{
		Comp: newNpSchedulerImpl(deps.EpForwarder),
	}
}

type npSchedulerImpl struct {
	epForwarder eventplatform.Component
}

func (s npSchedulerImpl) Schedule(hostname string, port uint16) {
	// TODO: IMPLEMENTATION IN SEPARATE PR (to make PRs easier to review)
}

func newNpSchedulerImpl(epForwarder eventplatform.Component) npSchedulerImpl {
	return npSchedulerImpl{
		epForwarder: epForwarder,
	}
}
