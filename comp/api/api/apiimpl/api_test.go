// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"context"
	"testing"

	// component dependencies

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"

	// package dependencies
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	// third-party dependencies
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

type testdeps struct {
	fx.In

	// additional StartServer arguments
	//
	// TODO: remove these in the next PR once StartServer component arguments
	//       are part of the api component dependency struct
	DogstatsdServer   dogstatsdServer.Component
	Capture           replay.Component
	SecretResolver    secrets.Component
	StatusComponent   status.Mock
	RcService         optional.Option[rcservice.Component]
	RcServiceMRF      optional.Option[rcservicemrf.Component]
	AuthToken         authtoken.Component
	WorkloadMeta      workloadmeta.Component
	Tagger            tagger.Mock
	Autodiscovery     autodiscovery.Mock
	Logs              optional.Option[logsAgent.Component]
	Collector         optional.Option[collector.Component]
	EndpointProviders []api.EndpointProvider `group:"agent_endpoint"`
}

func getComponentDependencies(t *testing.T) testdeps {
	// TODO: this fxutil.Test[T] can take a component and return the component
	return fxutil.Test[testdeps](
		t,
		dogstatsdServer.MockModule(),
		replaymock.MockModule(),
		secretsimpl.MockModule(),
		fx.Provide(func(secretMock secrets.Mock) secrets.Component {
			component := secretMock.(secrets.Component)
			return component
		}),
		statusimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[rcservice.Component]()),
		fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
		fetchonlyimpl.MockModule(),
		fx.Supply(context.Background()),
		taggerimpl.MockModule(),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[logsAgent.Component]()),
		fx.Supply(optional.NewNoneOption[collector.Component]()),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
	)
}

func getTestAPIServer(deps testdeps) api.Component {
	apideps := dependencies{
		DogstatsdServer:   deps.DogstatsdServer,
		Capture:           deps.Capture,
		SecretResolver:    deps.SecretResolver,
		StatusComponent:   deps.StatusComponent,
		RcService:         deps.RcService,
		RcServiceMRF:      deps.RcServiceMRF,
		AuthToken:         deps.AuthToken,
		Tagger:            deps.Tagger,
		LogsAgentComp:     deps.Logs,
		WorkloadMeta:      deps.WorkloadMeta,
		Collector:         deps.Collector,
		EndpointProviders: deps.EndpointProviders,
	}
	return newAPIServer(apideps)
}

func TestStartServer(t *testing.T) {
	deps := getComponentDependencies(t)

	sender := aggregator.NewNoOpSenderManager()

	srv := getTestAPIServer(deps)
	err := srv.StartServer(
		sender,
	)
	defer srv.StopServer()

	assert.NoError(t, err, "could not start api component servers: %v", err)
}
