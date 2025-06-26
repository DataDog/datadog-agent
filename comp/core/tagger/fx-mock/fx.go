// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the tagger component
package fx

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(taggerimpl.NewMock),
	)
}

// SetupFakeTagger calls fxutil.Test to create a mock tagger for testing
func SetupFakeTagger(t testing.TB) taggermock.Mock {
	return fxutil.Test[taggermock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		noopTelemetry.Module(),
		MockModule(),
		fx.Provide(func(t testing.TB) ipc.Component {
			return ipcmock.New(t)
		}),
	))
}
