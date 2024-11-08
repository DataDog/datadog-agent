// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"net/http"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// MockParams defines the parameters for the mock component.
type MockParams struct {
	Scheduler *scheduler.Controller
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
	LogsComp   log.Component
	Telemetry  telemetry.Component
	Secrets    secrets.Component
}

type mockprovides struct {
	fx.Out

	Comp     autodiscovery.Mock
	Endpoint api.AgentEndpointProvider
}

func newMockAutoConfig(deps mockdependencies) mockprovides {
	ac := createNewAutoConfig(deps.Params.Scheduler, deps.Secrets, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)
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
