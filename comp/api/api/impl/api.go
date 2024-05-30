// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// package impl implements the internal Agent API which exposes endpoints such as config, flare or status
package impl

import (
	"context"
	"net"

	"go.uber.org/fx"

	apidef "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type ApiServer struct {
	dogstatsdServer   dogstatsdServer.Component
	capture           replay.Component
	pidMap            pidmap.Component
	secretResolver    secrets.Component
	pkgSigning        packagesigning.Component
	statusComponent   status.Component
	rcService         optional.Option[rcservice.Component]
	rcServiceMRF      optional.Option[rcservicemrf.Component]
	authToken         authtoken.Component
	taggerComp        tagger.Component
	autoConfig        autodiscovery.Component
	logsAgentComp     optional.Option[logsAgent.Component]
	wmeta             workloadmeta.Component
	collector         optional.Option[collector.Component]
	senderManager     sender.DiagnoseSenderManager
	endpointProviders []apidef.EndpointProvider
}

type Requires struct {
	fx.In
	Lc                compdef.Lifecycle
	DogstatsdServer   dogstatsdServer.Component
	Capture           replay.Component
	PidMap            pidmap.Component
	SecretResolver    secrets.Component
	PkgSigning        packagesigning.Component
	StatusComponent   status.Component
	RcService         optional.Option[rcservice.Component]
	RcServiceMRF      optional.Option[rcservicemrf.Component]
	AuthToken         authtoken.Component
	Tagger            tagger.Component
	AutoConfig        autodiscovery.Component
	LogsAgentComp     optional.Option[logsAgent.Component]
	WorkloadMeta      workloadmeta.Component
	Collector         optional.Option[collector.Component]
	SenderManager     sender.DiagnoseSenderManager
	EndpointProviders []apidef.EndpointProvider `group:"agent_endpoint"`
}

var _ apidef.Component = (*ApiServer)(nil)

func NewAPIServer(reqs Requires) apidef.Component {
	server := ApiServer{
		dogstatsdServer:   reqs.DogstatsdServer,
		capture:           reqs.Capture,
		pidMap:            reqs.PidMap,
		secretResolver:    reqs.SecretResolver,
		pkgSigning:        reqs.PkgSigning,
		statusComponent:   reqs.StatusComponent,
		rcService:         reqs.RcService,
		rcServiceMRF:      reqs.RcServiceMRF,
		authToken:         reqs.AuthToken,
		taggerComp:        reqs.Tagger,
		autoConfig:        reqs.AutoConfig,
		logsAgentComp:     reqs.LogsAgentComp,
		wmeta:             reqs.WorkloadMeta,
		collector:         reqs.Collector,
		senderManager:     reqs.SenderManager,
		endpointProviders: fxutil.GetAndFilterGroup(reqs.EndpointProviders),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: server.StartServers,
		OnStop: func(_ context.Context) error {
			StopServers()
			return nil
		},
	})

	return &server
}

// StopServer closes the connection and the server
// stops listening to new commands.
func (server *ApiServer) StopServer() {
	StopServers()
}

// ServerAddress returns the server address.
func (server *ApiServer) ServerAddress() *net.TCPAddr {
	return ServerAddress()
}
