// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
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

type mockAPIServer struct {
}

var _ api.Mock = (*mockAPIServer)(nil)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)

func newMock() api.Mock {
	return &mockAPIServer{}
}

// StartServer creates the router and starts the HTTP server
func (mock *mockAPIServer) StartServer(
	_ *remoteconfig.Service,
	_ flare.Component,
	_ dogstatsdServer.Component,
	_ replay.Component,
	_ dogstatsddebug.Component,
	_ workloadmeta.Component,
	_ optional.Option[logsAgent.Component],
	_ sender.DiagnoseSenderManager,
	_ host.Component,
	_ inventoryagent.Component,
	_ demultiplexer.Component,
	_ inventoryhost.Component,
	_ secrets.Component,
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
