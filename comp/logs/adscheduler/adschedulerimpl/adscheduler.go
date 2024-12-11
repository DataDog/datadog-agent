// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package adschedulerimpl contains the AD scheduler implementation.
package adschedulerimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	logsadscheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newADScheduler),
	)
}

type dependencies struct {
	fx.In
	Autodiscovery autodiscovery.Component
}

func newADScheduler(deps dependencies) agentimpl.SchedulerProvider {
	scheduler := logsadscheduler.New(deps.Autodiscovery)
	return agentimpl.NewSchedulerProvider(scheduler)
}
