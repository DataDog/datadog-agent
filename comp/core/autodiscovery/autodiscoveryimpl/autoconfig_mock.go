// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"net/http"
	"testing"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// MockParams defines the parameters for the mock component.
type MockParams struct {
	Scheduler *scheduler.MetaScheduler
}

// mockHandleRequest is a simple mocked http.Handler function to test the route registers with the api component correctly
func (ac *AutoConfig) mockHandleRequest(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

type mockdependencies struct {
	fx.In
	WMeta      optional.Option[workloadmeta.Component]
	Params     MockParams
	TaggerComp tagger.Mock
}

type mockprovides struct {
	fx.Out

	Comp     autodiscovery.Mock
	Endpoint api.AgentEndpointProvider
}

func newMockAutoConfig(deps mockdependencies) mockprovides {
	ac := createNewAutoConfig(deps.Params.Scheduler, nil, deps.WMeta, deps.TaggerComp)
	return mockprovides{
		Comp:     ac,
		Endpoint: api.NewAgentEndpointProvider(ac.mockHandleRequest, "/config-check", "GET"),
	}
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
		taggerimpl.MockModule(),
		MockModule()))
}
