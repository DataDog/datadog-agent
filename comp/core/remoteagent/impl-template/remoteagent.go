// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package templateimpl implements the remoteagent component interface
package templateimpl

import (
	"net"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Requires defines the dependencies for the remoteagent component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Log       log.Component
	IPC       ipc.Component
	Config    config.Component
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
	registryAddress := net.JoinHostPort(reqs.Config.GetString("cmd_host"), reqs.Config.GetString("cmd_port"))

	remoteAgentServer, err := helper.NewUnimplementedRemoteAgentServer(reqs.IPC, reqs.Log, reqs.Config, reqs.Lifecycle, registryAddress, flavor.GetFlavor(), flavor.GetHumanReadableFlavor())
	if err != nil {
		return Provides{}, err
	}

	remoteagentImpl := &remoteagentImpl{
		log:               reqs.Log,
		ipc:               reqs.IPC,
		cfg:               reqs.Config,
		remoteAgentServer: remoteAgentServer,
	}

	// Add your gRPC services implementations here: e.g.
	// pbcore.RegisterStatusProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)

	provides := Provides{
		Comp: remoteagentImpl,
	}
	return provides, nil
}

type remoteagentImpl struct {
	log log.Component
	ipc ipc.Component
	cfg config.Component

	remoteAgentServer *helper.UnimplementedRemoteAgentServer
}
