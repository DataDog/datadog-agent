// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl implements the internal Agent API which exposes endpoints such as config, flare or status
package apiimpl

import (
	"context"
	"net"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAPIServer))
}

type apiServer struct {
	cfg               config.Component
	authToken         authtoken.Component
	cmdListener       net.Listener
	ipcListener       net.Listener
	telemetry         telemetry.Component
	endpointProviders []api.EndpointProvider
	grpcComponent     grpc.Component
}

type dependencies struct {
	fx.In

	Lc                fx.Lifecycle
	AuthToken         authtoken.Component
	Cfg               config.Component
	Telemetry         telemetry.Component
	EndpointProviders []api.EndpointProvider `group:"agent_endpoint"`
	GrpcComponent     grpc.Component
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer(deps dependencies) api.Component {

	server := apiServer{
		authToken:         deps.AuthToken,
		cfg:               deps.Cfg,
		telemetry:         deps.Telemetry,
		endpointProviders: fxutil.GetAndFilterGroup(deps.EndpointProviders),
		grpcComponent:     deps.GrpcComponent,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error { return server.startServers() },
		OnStop: func(_ context.Context) error {
			server.stopServers()
			return nil
		},
	})

	return &server
}

// ServerAddress returns the server address.
func (server *apiServer) CMDServerAddress() *net.TCPAddr {
	return server.cmdListener.Addr().(*net.TCPAddr)
}

// ServerAddress returns the server address.
func (server *apiServer) IPCServerAddress() *net.TCPAddr {
	return server.ipcListener.Addr().(*net.TCPAddr)
}
