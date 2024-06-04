// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"context"
	"testing"

	// component dependencies
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning/packagesigningimpl"
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
	DogstatsdServer       dogstatsdServer.Component
	Capture               replay.Component
	ServerDebug           dogstatsddebug.Component
	Demux                 demultiplexer.Component
	SecretResolver        secrets.Component
	PkgSigning            packagesigning.Component
	StatusComponent       status.Mock
	EventPlatformReceiver eventplatformreceiver.Component
	RcService             optional.Option[rcservice.Component]
	RcServiceMRF          optional.Option[rcservicemrf.Component]
	AuthToken             authtoken.Component
	WorkloadMeta          workloadmeta.Component
	Tagger                tagger.Mock
	Autodiscovery         autodiscovery.Mock
	Logs                  optional.Option[logsAgent.Component]
	Collector             optional.Option[collector.Component]
	EndpointProviders     []api.EndpointProvider `group:"agent_endpoint"`
}

func getComponentDependencies(t *testing.T) testdeps {
	// TODO: this fxutil.Test[T] can take a component and return the component
	return fxutil.Test[testdeps](
		t,
		hostnameimpl.MockModule(),
		flareimpl.MockModule(),
		dogstatsdServer.MockModule(),
		replaymock.MockModule(),
		serverdebugimpl.MockModule(),
		hostimpl.MockModule(),
		inventoryagentimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		inventoryhostimpl.MockModule(),
		secretsimpl.MockModule(),
		nooptelemetry.Module(),
		inventorychecksimpl.MockModule(),
		packagesigningimpl.MockModule(),
		statusimpl.MockModule(),
		eventplatformreceiverimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[rcservice.Component]()),
		fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
		fetchonlyimpl.MockModule(),
		fx.Supply(context.Background()),
		taggerimpl.MockModule(),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Provide(func() optional.Option[logsAgent.Component] {
			return optional.NewNoneOption[logsAgent.Component]()
		}),
		fx.Provide(func() optional.Option[collector.Component] {
			return optional.NewNoneOption[collector.Component]()
		}),
		settingsimpl.MockModule(),
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
		PkgSigning:        deps.PkgSigning,
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
