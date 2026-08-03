// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build (test || functionaltests || stresstests) && !serverless

package telemetryimpl

import (
	"context"

	"go.uber.org/fx"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule provides a telemetry mock module for use within this package's tests.
// This cannot use comp/core/telemetry/mock to avoid a circular import: mock imports telemetryimpl.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func(lc fx.Lifecycle) telemetry.Mock {
			tel := NewMockComponent()
			lc.Append(fx.Hook{
				OnStop: func(_ context.Context) error {
					tel.Reset()
					return nil
				},
			})
			return tel
		}),
		fx.Provide(func(m telemetry.Mock) telemetry.Component { return m }),
	)
}
