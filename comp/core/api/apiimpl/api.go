// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"net"

	"go.uber.org/fx"

	apiPackage "github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/comp/core/api"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newAPIServer),
)

type apiServer struct {
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer() api.Component {
	return &apiServer{}
}

// StartServer creates the router and starts the HTTP server
func (server *apiServer) StartServer(
	configService *remoteconfig.Service,
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	invHost inventoryhost.Component,
) error {
	return apiPackage.StartServer(configService,
		flare,
		dogstatsdServer,
		capture,
		serverDebug,
		wmeta,
		logsAgent,
		senderManager,
		hostMetadata,
		invAgent,
		invHost,
	)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func (server *apiServer) StopServer() {
	apiPackage.StopServer()
}

// ServerAddress returns the server address.
func (server *apiServer) ServerAddress() *net.TCPAddr {
	return apiPackage.ServerAddress()
}
