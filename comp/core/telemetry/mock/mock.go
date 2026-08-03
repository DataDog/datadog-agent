// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build (test || functionaltests || stresstests) && !serverless

// Package mock provides the mock for the telemetry component.
package mock

import (
	"context"
	"testing"

	"go.uber.org/fx"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDependencies struct {
	fx.In

	Lifecycle fx.Lifecycle
}

func newMockComponent(deps testDependencies) telemetry.Mock {
	tel := telemetryimpl.NewMockComponent()
	deps.Lifecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			tel.Reset()
			return nil
		},
	})
	return tel
}

// Module defines the fx options for the mock telemetry component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockComponent),
		fx.Provide(func(m telemetry.Mock) telemetry.Component { return m }),
	)
}

// New returns a new mock telemetry component for testing.
func New(t testing.TB) telemetry.Mock {
	return telemetryimpl.NewMock(t)
}
