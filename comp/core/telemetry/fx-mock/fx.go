// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test || functionaltests || stresstests

// Package fx provides the fx module for the mock telemetry component
package fx

import (
	"context"

	"go.uber.org/fx"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(telemetryimpl.NewMockNoCleanup),
		fx.Provide(func(m telemetryimpl.Mock) telemetry.Component { return m }),
		fx.Invoke(setupLifecycle),
	)
}

func setupLifecycle(lc fx.Lifecycle, mock telemetryimpl.Mock) {
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			mock.Reset()
			return nil
		},
	})
}
