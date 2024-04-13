// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package schedulerimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/networkpath/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	// populate the component dependencies
}

type provides struct {
	fx.Out

	Comp scheduler.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newScheduler),
	)
}

func newScheduler(deps dependencies) provides {
	// Component initialization
	return provides{
		Comp: newSchedulerImpl(),
	}
}

type schedulerImpl struct {
}

func (s schedulerImpl) SchedulePath() {
	//TODO implement me
	log.Error("SchedulePath called")
}

func newSchedulerImpl() schedulerImpl {
	return schedulerImpl{}
}
