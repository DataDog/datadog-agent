// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	logslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// GetWorkloadMetaMock returns a mock of the workloadmeta.Component.
func GetWorkloadMetaMock(t testing.TB) workloadmetamock.Mock {
	opts := []fx.Option{
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	}

	// If the test is a fuzz test, the logger provided in core.MockBundle() will be created with the wrong testing.TB
	// and cause a panic.
	if _, ok := t.(*testing.F); ok {
		// fx.Decorate allows transforming a given component, in this case we replace it with a disabled logger
		opts = append(opts, fx.Decorate(func(log.Component) log.Component { return logslog.Disabled() }))
	}

	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(opts...))
}

// GetWorkloadMetaMockWithDefaultGPUs is the same as GetWorkloadMetaMock, but adds the GPUs of testutil.GPUUUIDs
func GetWorkloadMetaMockWithDefaultGPUs(t testing.TB) workloadmetamock.Mock {
	wmeta := GetWorkloadMetaMock(t)
	for _, uuid := range GPUUUIDs {
		wmeta.Set(&workloadmeta.GPU{
			EntityID: workloadmeta.EntityID{
				ID:   uuid,
				Kind: workloadmeta.KindGPU,
			},
		})
	}
	return wmeta
}

// GetTelemetryMock returns a mock of the telemetry.Component.
func GetTelemetryMock(t testing.TB) telemetry.Mock {
	return fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
}
