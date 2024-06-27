// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package telemetryimpl

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDependencies struct {
	fx.In

	Lyfecycle fx.Lifecycle
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Provide(func(m telemetry.Mock) telemetry.Component { return m }))
}

type telemetryImplMock struct {
	telemetryImpl
}

func newMock(deps testDependencies) telemetry.Mock {
	reg := prometheus.NewRegistry()
	provider := newProvider(reg)

	telemetry := &telemetryImplMock{
		telemetryImpl{
			mutex:         &mutex,
			registry:      reg,
			meterProvider: provider,
		},
	}

	deps.Lyfecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			telemetry.Reset()

			return nil
		},
	})

	return telemetry
}

func (t *telemetryImplMock) GetRegistry() *prometheus.Registry {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.registry
}

func (t *telemetryImplMock) GetMeterProvider() *sdk.MeterProvider {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.meterProvider
}
