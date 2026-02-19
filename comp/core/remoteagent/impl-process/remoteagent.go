// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package processimpl implements the remoteagent component interface
package processimpl

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
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
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
	Telemetry telemetry.Component
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
		telemetry:         reqs.Telemetry,
		remoteAgentServer: remoteAgentServer,
	}

	// Add your gRPC services implementations here:
	pbcore.RegisterTelemetryProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
	pbcore.RegisterStatusProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
	pbcore.RegisterFlareProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)

	provides := Provides{
		Comp: remoteagentImpl,
	}
	return provides, nil
}

type remoteagentImpl struct {
	log       log.Component
	ipc       ipc.Component
	cfg       config.Component
	telemetry telemetry.Component

	remoteAgentServer *helper.UnimplementedRemoteAgentServer
	pbcore.UnimplementedTelemetryProviderServer
	pbcore.UnimplementedStatusProviderServer
	pbcore.UnimplementedFlareProviderServer
}

func (r *remoteagentImpl) GetTelemetry(_ context.Context, _ *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	prometheusText, err := r.telemetry.GatherText(false, telemetry.StaticMetricFilter(
	// Add here the metric names that should be included in the telemetry response.
	// This is useful to avoid sending too many metrics to the Core Agent.
	))
	if err != nil {
		return nil, err
	}

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: prometheusText,
		},
	}, nil
}

// GetStatusDetails returns the status details of the process agent
func (r *remoteagentImpl) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	fields := make(map[string]string)
	expvar.Do(func(kv expvar.KeyValue) {
		fields[kv.Key] = kv.Value.String()
	})
	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{Fields: fields},
	}, nil
}

// GetFlareFiles returns files for the process agent flare
func (r *remoteagentImpl) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	files := make(map[string][]byte)

	// structured JSON
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
		files["process_agent_status.json"] = data
	}

	if data, err := json.MarshalIndent(r.cfg.AllSettings(), "", "  "); err == nil {
		files["process_agent_runtime_config_dump.json"] = data
	}

	return &pbcore.GetFlareFilesResponse{Files: files}, nil
}
