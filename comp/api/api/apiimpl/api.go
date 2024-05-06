// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"net"

	"go.uber.org/fx"

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
	dogstatsdServer   dogstatsdServer.Component
	capture           replay.Component
	pidMap            pidmap.Component
	hostMetadata      host.Component
	invAgent          inventoryagent.Component
	invHost           inventoryhost.Component
	secretResolver    secrets.Component
	invChecks         inventorychecks.Component
	pkgSigning        packagesigning.Component
	statusComponent   status.Component
	rcService         optional.Option[rcservice.Component]
	rcServiceMRF      optional.Option[rcservicemrf.Component]
	authToken         authtoken.Component
	gui               optional.Option[gui.Component]
	settings          settings.Component
	endpointProviders []api.EndpointProvider
}

type dependencies struct {
	fx.In

	DogstatsdServer   dogstatsdServer.Component
	Capture           replay.Component
	PidMap            pidmap.Component
	HostMetadata      host.Component
	InvAgent          inventoryagent.Component
	InvHost           inventoryhost.Component
	SecretResolver    secrets.Component
	InvChecks         inventorychecks.Component
	PkgSigning        packagesigning.Component
	StatusComponent   status.Component
	RcService         optional.Option[rcservice.Component]
	RcServiceMRF      optional.Option[rcservicemrf.Component]
	AuthToken         authtoken.Component
	Gui               optional.Option[gui.Component]
	Settings          settings.Component
	EndpointProviders []api.EndpointProvider `group:"agent_endpoint"`
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer(deps dependencies) api.Component {
	return &apiServer{
		dogstatsdServer:   deps.DogstatsdServer,
		capture:           deps.Capture,
		pidMap:            deps.PidMap,
		hostMetadata:      deps.HostMetadata,
		invAgent:          deps.InvAgent,
		invHost:           deps.InvHost,
		secretResolver:    deps.SecretResolver,
		invChecks:         deps.InvChecks,
		pkgSigning:        deps.PkgSigning,
		statusComponent:   deps.StatusComponent,
		rcService:         deps.RcService,
		rcServiceMRF:      deps.RcServiceMRF,
		authToken:         deps.AuthToken,
		gui:               deps.Gui,
		settings:          deps.Settings,
		endpointProviders: deps.EndpointProviders,
	}
}

// StartServer creates the router and starts the HTTP server
func (server *apiServer) StartServer(
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	collector optional.Option[collector.Component],
) error {
	return StartServers(server.rcService,
		server.rcServiceMRF,
		server.dogstatsdServer,
		server.capture,
		server.pidMap,
		wmeta,
		taggerComp,
		logsAgent,
		senderManager,
		server.hostMetadata,
		server.invAgent,
		server.invHost,
		server.secretResolver,
		server.invChecks,
		server.pkgSigning,
		server.statusComponent,
		collector,
		ac,
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
