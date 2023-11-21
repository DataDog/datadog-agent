// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
func (server *apiServer) StartServer(configService *remoteconfig.Service,
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	logsAgent pkgUtil.Optional[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
) error {
	return api.StartServer(configService,
		flare,
		dogstatsdServer,
		capture,
		serverDebug,
		logsAgent,
		senderManager,
		hostMetadata,
		invAgent,
	)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func (server *apiServer) StopServer() {
	return api.StopServer()
}

// ServerAddress returns the server address.
func (server *apiServer) ServerAddress() *net.TCPAddr {
	return api.ServerAddress()
}
