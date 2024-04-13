// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	// populate the component dependencies
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
		Comp: newNpSchedulerImpl(),
	}
}

type npSchedulerImpl struct {
}

func (s npSchedulerImpl) Schedule() {
	//TODO implement me
	log.Error("Schedule called")
}

func newNpSchedulerImpl() npSchedulerImpl {
	return npSchedulerImpl{}
}
