// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package securityagentimpl implements the remoteagent component interface
package securityagentimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"net"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	"github.com/DataDog/datadog-agent/comp/core/status"
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
	Status    status.Component
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
		statusComp:        reqs.Status,
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
	log        log.Component
	ipc        ipc.Component
	cfg        config.Component
	statusComp status.Component

	remoteAgentServer *helper.UnimplementedRemoteAgentServer
	pbcore.UnimplementedTelemetryProviderServer
	pbcore.UnimplementedStatusProviderServer
	pbcore.UnimplementedFlareProviderServer
}

// GetStatusDetails returns the status details of the security agent
func (r *remoteagentImpl) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	mainFields := make(map[string]string)

	// expvar data
	expvar.Do(func(kv expvar.KeyValue) {
		mainFields[kv.Key] = kv.Value.String()
	})

	if statusJSON, err := r.statusComp.GetStatus("json", false); err == nil {
		mainFields["status"] = string(statusJSON)
	}

	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{Fields: mainFields},
	}, nil
}

// GetFlareFiles returns files for the security agent flare
func (r *remoteagentImpl) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	files := make(map[string][]byte)

	if statusJSON, err := r.statusComp.GetStatus("json", false); err == nil {
		files["security_agent_status.json"] = statusJSON
	}

	expvarData := make(map[string]any)
	expvar.Do(func(kv expvar.KeyValue) {
		var v any
		if err := json.Unmarshal([]byte(kv.Value.String()), &v); err == nil {
			expvarData[kv.Key] = v
		} else {
			expvarData[kv.Key] = kv.Value.String()
		}
	})
	if data, err := json.MarshalIndent(expvarData, "", "  "); err == nil {
		files["security_agent_expvar_dump.json"] = data
	}

	if data, err := json.MarshalIndent(r.cfg.AllSettings(), "", "  "); err == nil {
		files["security_agent_runtime_config_dump.json"] = data
	}

	return &pbcore.GetFlareFilesResponse{Files: files}, nil
}
