// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl implements the remoteagent component interface
package impl

import (
	"net"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Requires defines the dependencies for the remoteagent component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Log       log.Component
	IPC       ipc.Component
	Config    config.Component
	// Telemetry is optional - only used by system-probe for COAT metrics
	Telemetry telemetry.Component `optional:"true"`
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
	registryAddress := net.JoinHostPort(reqs.Config.GetString("cmd_host"), reqs.Config.GetString("cmd_port"))

	register, err := helper.NewUnimplementedRemoteAgentServer(reqs.IPC, reqs.Log, reqs.Config, reqs.Lifecycle, registryAddress, flavor.GetFlavor(), flavor.GetHumanReadableFlavor())
	if err != nil {
		return Provides{}, err
	}

	// Register telemetry service if telemetry component is available (system-probe case)
	if reqs.Telemetry != nil {
		telemetryProvider := newTelemetryProvider(reqs.Telemetry)
		pbcore.RegisterTelemetryProviderServer(register.GetGRPCServer(), telemetryProvider)
		reqs.Log.Info("Registered telemetry service with RemoteAgent")
	}

	remoteagentImpl := &remoteagentImpl{
		log:      reqs.Log,
		ipc:      reqs.IPC,
		cfg:      reqs.Config,
		register: register,
	}

	provides := Provides{
		Comp: remoteagentImpl,
	}
	return provides, nil
}

type remoteagentImpl struct {
	log log.Component
	ipc ipc.Component
	cfg config.Component

	register *helper.UnimplementedRemoteAgentServer
}

// GetGRPCServer returns the gRPC server for service registration
func (r *remoteagentImpl) GetGRPCServer() *grpc.Server {
	return r.register.GetGRPCServer()
}
