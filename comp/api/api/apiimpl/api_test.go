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
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning/packagesigningimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha"

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

	Flare                 flare.Component
	DogstatsdServer       dogstatsdServer.Component
	Capture               replay.Component
	ServerDebug           dogstatsddebug.Component
	HostMetadata          host.Component
	InvAgent              inventoryagent.Component
	Demux                 demultiplexer.Component
	InvHost               inventoryhost.Component
	SecretResolver        secrets.Component
	InvChecks             inventorychecks.Component
	PkgSigning            packagesigning.Component
	StatusComponent       status.Mock
	EventPlatformReceiver eventplatformreceiver.Component
	RcService             optional.Option[rcservice.Component]
	RcServiceHA           optional.Option[rcserviceha.Component]
	AuthToken             authtoken.Component
	WorkloadMeta          workloadmeta.Component
	Tagger                tagger.Mock
	Autodiscovery         autodiscovery.Mock
	Logs                  optional.Option[logsAgent.Component]
	Collector             optional.Option[collector.Component]
}

func getComponentDependencies(t *testing.T) testdeps {
	return fxutil.Test[testdeps](
		t,
		flareimpl.MockModule(),
		dogstatsdServer.MockModule(),
		replay.MockModule(),
		serverdebugimpl.MockModule(),
		hostimpl.MockModule(),
		inventoryagentimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		inventoryhostimpl.MockModule(),
		secretsimpl.MockModule(),
		fx.Provide(func(secretMock secrets.Mock) secrets.Component {
			component := secretMock.(secrets.Component)
			return component
		}),
		inventorychecksimpl.MockModule(),
		packagesigningimpl.MockModule(),
		statusimpl.MockModule(),
		eventplatformreceiverimpl.MockModule(),
		fx.Provide(func() optional.Option[rcservice.Component] {
			return optional.NewNoneOption[rcservice.Component]()
		}),
		fx.Provide(func() optional.Option[rcserviceha.Component] {
			return optional.NewNoneOption[rcserviceha.Component]()
		}),
		fetchonlyimpl.MockModule(),
		fx.Supply(context.Background()),
		tagger.MockModule(),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Provide(func() optional.Option[logsAgent.Component] {
			return optional.NewNoneOption[logsAgent.Component]()
		}),
		fx.Provide(func() optional.Option[collector.Component] {
			return optional.NewNoneOption[collector.Component]()
		}),
	)
}

func getTestAPIServer(deps testdeps) *apiServer {
	apideps := dependencies{
		Flare:                 deps.Flare,
		DogstatsdServer:       deps.DogstatsdServer,
		Capture:               deps.Capture,
		ServerDebug:           deps.ServerDebug,
		HostMetadata:          deps.HostMetadata,
		InvAgent:              deps.InvAgent,
		Demux:                 deps.Demux,
		InvHost:               deps.InvHost,
		SecretResolver:        deps.SecretResolver,
		InvChecks:             deps.InvChecks,
		PkgSigning:            deps.PkgSigning,
		StatusComponent:       deps.StatusComponent,
		EventPlatformReceiver: deps.EventPlatformReceiver,
		RcService:             deps.RcService,
		RcServiceHA:           deps.RcServiceHA,
		AuthToken:             deps.AuthToken,
	}
	api := newAPIServer(apideps)
	return api.(*apiServer)
}

func TestStartServer(t *testing.T) {
	deps := getComponentDependencies(t)

	store := deps.WorkloadMeta
	tags := deps.Tagger
	ac := deps.Autodiscovery
	sender := aggregator.NewNoOpSenderManager()
	log := deps.Logs
	col := deps.Collector

	srv := getTestAPIServer(deps)
	err := srv.StartServer(
		store,
		tags,
		ac,
		log,
		sender,
		col,
	)
	defer srv.StopServer()

	assert.NoError(t, err, "could not start api component servers: %v", err)
}
