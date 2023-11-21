// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package api implements the internal Agent API which exposes endpoints such as config, flare or status
package api

import (
	"net"

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
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	StartServer(configService *remoteconfig.Service,
		flare flare.Component,
		dogstatsdServer dogstatsdServer.Component,
		capture replay.Component,
		serverDebug dogstatsddebug.Component,
		logsAgent pkgUtil.Optional[logsAgent.Component],
		senderManager sender.DiagnoseSenderManager,
		hostMetadata host.Component,
		invAgent inventoryagent.Component,
	) error
	StopServer()
	ServerAddress() *net.TCPAddr
}
