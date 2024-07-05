// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
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
	secretResolver    secrets.Component
	rcService         optional.Option[rcservice.Component]
	rcServiceMRF      optional.Option[rcservicemrf.Component]
	authToken         authtoken.Component
	taggerComp        tagger.Component
	autoConfig        autodiscovery.Component
	logsAgentComp     optional.Option[logsAgent.Component]
	wmeta             workloadmeta.Component
	collector         optional.Option[collector.Component]
	senderManager     diagnosesendermanager.Component
	telemetry         telemetry.Component
	endpointProviders []api.EndpointProvider
}

type dependencies struct {
	fx.In

	DogstatsdServer       dogstatsdServer.Component
	Capture               replay.Component
	PidMap                pidmap.Component
	SecretResolver        secrets.Component
	RcService             optional.Option[rcservice.Component]
	RcServiceMRF          optional.Option[rcservicemrf.Component]
	AuthToken             authtoken.Component
	Tagger                tagger.Component
	AutoConfig            autodiscovery.Component
	LogsAgentComp         optional.Option[logsAgent.Component]
	WorkloadMeta          workloadmeta.Component
	Collector             optional.Option[collector.Component]
	DiagnoseSenderManager diagnosesendermanager.Component
	Telemetry             telemetry.Component
	EndpointProviders     []api.EndpointProvider `group:"agent_endpoint"`
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer(deps dependencies) api.Component {
	return &apiServer{
		dogstatsdServer:   deps.DogstatsdServer,
		capture:           deps.Capture,
		pidMap:            deps.PidMap,
		secretResolver:    deps.SecretResolver,
		rcService:         deps.RcService,
		rcServiceMRF:      deps.RcServiceMRF,
		authToken:         deps.AuthToken,
		taggerComp:        deps.Tagger,
		autoConfig:        deps.AutoConfig,
		logsAgentComp:     deps.LogsAgentComp,
		wmeta:             deps.WorkloadMeta,
		collector:         deps.Collector,
		senderManager:     deps.DiagnoseSenderManager,
		telemetry:         deps.Telemetry,
		endpointProviders: fxutil.GetAndFilterGroup(deps.EndpointProviders),
	}
}

// StartServer creates the router and starts the HTTP server
func (server *apiServer) StartServer() error {
	return StartServers(
		server.rcService,
		server.rcServiceMRF,
		server.dogstatsdServer,
		server.capture,
		server.pidMap,
		server.wmeta,
		server.taggerComp,
		server.logsAgentComp,
		server.senderManager,
		server.secretResolver,
		server.collector,
		server.autoConfig,
		server.endpointProviders,
		server.telemetry,
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
