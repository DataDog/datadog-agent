// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/trace/config"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Shutdowner fx.Shutdowner

	Ctx                context.Context
	Config             config.Component
	TelemetryCollector telemetry.TelemetryCollector
}

type agent struct {
	*pkgagent.Agent

	// TODO(AIT-8301): What is this used for?
	shutdowner fx.Shutdowner
}

func newAgent(deps dependencies) Component {
	// fx init
	ag := &agent{
		Agent: pkgagent.NewAgent(
			deps.Ctx,
			deps.Config.Object(),
			deps.TelemetryCollector,
		),
		shutdowner: deps.Shutdowner,
	}

	// fx lifecycle hooks
	deps.Lc.Append(fx.Hook{OnStart: ag.start, OnStop: ag.stop})

	return ag
}

// Start hook has a fx enforced timeout, so don't do long-running operations.
func (ag *agent) start(ctx context.Context) error {
	return nil
}

func (ag *agent) stop(ctx context.Context) error {
	return nil
}
