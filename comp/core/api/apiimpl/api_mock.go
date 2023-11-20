// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/api"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgUtil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockApiServer struct {
	name string
}

var _ api.Mock = (*mockApiServer)(nil)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)

func newMock() api.Mock {
	return &mockApiServer{}
}

func (mock *mockApiServer) StartServer(
	configService *remoteconfig.Service,
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	logsAgent pkgUtil.Optional[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
) error {
	return nil
}

func (mock *mockApiServer) StopServer() {
}

// ServerAddress retruns the server address.
func (mock *mockApiServer) ServerAddress() *net.TCPAddr {
	return nil
}
