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
	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	filterimpl "github.com/DataDog/datadog-agent/comp/core/filter/impl"
	filtermock "github.com/DataDog/datadog-agent/comp/core/filter/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(filterimpl.NewMock),
		fx.Provide(func(mock filtermock.Mock) filter.Component { return mock }),
	)
}

// SetupMockFilter calls fxutil.Test to create a mock filter for testing
func SetupMockFilter(t testing.TB) filtermock.Mock {
	return fxutil.Test[filtermock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		noopTelemetry.Module(),
		MockModule(),
	))
}
