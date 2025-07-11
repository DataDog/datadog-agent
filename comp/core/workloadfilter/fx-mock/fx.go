// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the filter component
package fx

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl"
	workloadfiltermock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(workloadfilterimpl.NewMock),
		fx.Provide(func(mock workloadfiltermock.Mock) workloadfilter.Component { return mock }),
	)
}

// SetupMockFilter calls fxutil.Test to create a mock filter for testing
func SetupMockFilter(t testing.TB) workloadfiltermock.Mock {
	return fxutil.Test[workloadfiltermock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		noopTelemetry.Module(),
		MockModule(),
	))
}
