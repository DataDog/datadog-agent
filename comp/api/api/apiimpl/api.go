// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAPIServer))
}

type apiServer struct {
	dogstatsdServer       dogstatsdServer.Component
	capture               replay.Component
	pidMap                pidmap.Component
	serverDebug           dogstatsddebug.Component
	hostMetadata          host.Component
	invAgent              inventoryagent.Component
	demux                 demultiplexer.Component
	invHost               inventoryhost.Component
	secretResolver        secrets.Component
	invChecks             inventorychecks.Component
	pkgSigning            packagesigning.Component
	statusComponent       status.Component
	eventPlatformReceiver eventplatformreceiver.Component
	rcService             optional.Option[rcservice.Component]
	rcServiceMRF          optional.Option[rcservicemrf.Component]
	authToken             authtoken.Component
	taggerComp            tagger.Component
	autoConfig            autodiscovery.Component
	logsAgentComp         optional.Option[logsAgent.Component]
	wmeta                 workloadmeta.Component
	collector             optional.Option[collector.Component]
	gui                   optional.Option[gui.Component]
	settings              settings.Component
	endpointProviders     []api.EndpointProvider
}

type dependencies struct {
	fx.In

	DogstatsdServer       dogstatsdServer.Component
	Capture               replay.Component
	PidMap                pidmap.Component
	ServerDebug           dogstatsddebug.Component
	HostMetadata          host.Component
	InvAgent              inventoryagent.Component
	Demux                 demultiplexer.Component
	InvHost               inventoryhost.Component
	SecretResolver        secrets.Component
	InvChecks             inventorychecks.Component
	PkgSigning            packagesigning.Component
	StatusComponent       status.Component
	EventPlatformReceiver eventplatformreceiver.Component
	RcService             optional.Option[rcservice.Component]
	RcServiceMRF          optional.Option[rcservicemrf.Component]
	AuthToken             authtoken.Component
	Tagger                tagger.Component
	AutoConfig            autodiscovery.Component
	LogsAgentComp         optional.Option[logsAgent.Component]
	WorkloadMeta          workloadmeta.Component
	Collector             optional.Option[collector.Component]
	Gui                   optional.Option[gui.Component]
	Settings              settings.Component
	EndpointProviders     []api.EndpointProvider `group:"agent_endpoint"`
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer(deps dependencies) api.Component {
	return &apiServer{
		dogstatsdServer:       deps.DogstatsdServer,
		capture:               deps.Capture,
		pidMap:                deps.PidMap,
		serverDebug:           deps.ServerDebug,
		hostMetadata:          deps.HostMetadata,
		invAgent:              deps.InvAgent,
		demux:                 deps.Demux,
		invHost:               deps.InvHost,
		secretResolver:        deps.SecretResolver,
		invChecks:             deps.InvChecks,
		pkgSigning:            deps.PkgSigning,
		statusComponent:       deps.StatusComponent,
		eventPlatformReceiver: deps.EventPlatformReceiver,
		rcService:             deps.RcService,
		rcServiceMRF:          deps.RcServiceMRF,
		authToken:             deps.AuthToken,
		taggerComp:            deps.Tagger,
		autoConfig:            deps.AutoConfig,
		logsAgentComp:         deps.LogsAgentComp,
		wmeta:                 deps.WorkloadMeta,
		collector:             deps.Collector,
		gui:                   deps.Gui,
		settings:              deps.Settings,
		endpointProviders:     deps.EndpointProviders,
	}
}

// StartServer creates the router and starts the HTTP server
func (server *apiServer) StartServer(
	senderManager sender.DiagnoseSenderManager,
) error {
	return StartServers(server.rcService,
		server.rcServiceMRF,
		server.dogstatsdServer,
		server.capture,
		server.pidMap,
		server.serverDebug,
		server.wmeta,
		server.taggerComp,
		server.logsAgentComp,
		senderManager,
		server.demux,
		server.secretResolver,
		server.statusComponent,
		server.collector,
		server.eventPlatformReceiver,
		server.autoConfig,
		server.gui,
		server.settings,
		server.endpointProviders,
	)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func (server *apiServer) StopServer() {
	StopServers()
}

// ServerAddress returns the server address.
func (server *apiServer) ServerAddress() *net.TCPAddr {
	return ServerAddress()
}
