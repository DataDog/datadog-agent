// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package traceimpl implements the remoteagent component interface
package traceimpl

import (
	"context"
	"encoding/json"
	"net"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Requires defines the dependencies for the remoteagent component
type Requires struct {
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
	// Check if the remoteAgentRegistry is enabled
	if !reqs.Config.GetBool("remote_agent_registry.enabled") {
		return Provides{}, nil
	}

	// Get the registry address
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

	pbcore.RegisterStatusProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
	pbcore.RegisterFlareProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)

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
	pbcore.UnimplementedTelemetryProviderServer
	pbcore.UnimplementedStatusProviderServer
	pbcore.UnimplementedFlareProviderServer
}

// GetStatusDetails returns the status details of the trace agent
func (r *remoteagentImpl) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{Fields: helper.ExpvarFields()},
	}, nil
}

// GetFlareFiles returns files for the trace agent flare
func (r *remoteagentImpl) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	files := make(map[string][]byte)

	if data, err := json.MarshalIndent(helper.ExpvarData(), "", "  "); err == nil {
		files["trace_agent_status.json"] = data
	}

	if data, err := json.MarshalIndent(r.cfg.AllSettings(), "", "  "); err == nil {
		files["trace_agent_runtime_config_dump.json"] = data
	}

	return &pbcore.GetFlareFilesResponse{Files: files}, nil
}
