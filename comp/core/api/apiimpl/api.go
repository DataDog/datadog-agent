// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package apiimpl ... /* TODO: detailed doc comment for the component */
package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newAPIServer),
)

type apiServer struct {
	listener net.Listener
}

var _ api.Component = (*apiServer)(nil)

func newAPIServer() api.Component {
	return &apiServer{}
}

// ServerAddress retruns the server address.
func (server *apiServer) ServerAddress() *net.TCPAddr {
	return server.listener.Addr().(*net.TCPAddr)
}
