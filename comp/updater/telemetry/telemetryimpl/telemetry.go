// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetryimpl provides the telemetry component implementation.
package telemetryimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Config config.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetry),
	)
}

func newTelemetry(deps dependencies) (telemetry.Component, error) {
	telemetry, err := daemon.NewTelemetry(deps.Config)
	if err != nil {
		return nil, err
	}
	deps.Lc.Append(fx.Hook{OnStart: telemetry.Start, OnStop: telemetry.Stop})
	return telemetry, nil
}
