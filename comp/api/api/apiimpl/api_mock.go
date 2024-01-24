// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type mockAPIServer struct {
}

var _ api.Mock = (*mockAPIServer)(nil)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

func newMock() api.Mock {
	return &mockAPIServer{}
}

// StartServer creates the router and starts the HTTP server
func (mock *mockAPIServer) StartServer(
	_ *remoteconfig.Service,
	_ workloadmeta.Component,
	_ tagger.Component,
	_ optional.Option[logsAgent.Component],
	_ sender.DiagnoseSenderManager,
) error {
	return nil
}

// StopServer closes the connection and the server
// stops listening to new commands.
func (mock *mockAPIServer) StopServer() {
}

// ServerAddress returns the server address.
func (mock *mockAPIServer) ServerAddress() *net.TCPAddr {
	return nil
}
