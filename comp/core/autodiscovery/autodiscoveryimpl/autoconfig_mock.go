// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// MockParams defines the parameters for the mock component.
type MockParams struct {
	Scheduler *scheduler.MetaScheduler
}

type mockdependencies struct {
	fx.In
	WMeta  optional.Option[workloadmeta.Component]
	Params MockParams
}

func newMockAutoConfig(deps mockdependencies) autodiscovery.Mock {
	return createNewAutoConfig(deps.Params.Scheduler, nil, deps.WMeta)
}

// MockModule provides the default autoconfig without other components configured, and not started
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockAutoConfig),
	)
}

// CreateMockAutoConfig creates a mock AutoConfig for testing
func CreateMockAutoConfig(t *testing.T, scheduler *scheduler.MetaScheduler) autodiscovery.Mock {
	return fxutil.Test[autodiscovery.Mock](t, fx.Options(
		fx.Supply(MockParams{Scheduler: scheduler}),
		MockModule()))
}
