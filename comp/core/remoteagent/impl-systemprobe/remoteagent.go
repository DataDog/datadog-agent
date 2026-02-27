// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package systemprobeimpl implements the remoteagent component interface
package systemprobeimpl

import (
	"context"
	"net"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
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

	// Set the agent identity for log metrics partitioning so that
	// logs.bytes_sent is tagged with remote_agent="system-probe".
	metrics.SetAgentIdentity("system-probe")

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
}

func (r *remoteagentImpl) GetTelemetry(_ context.Context, _ *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	prometheusText, err := r.telemetry.GatherText(false, telemetry.StaticMetricFilter(
		// Metrics to forward from system-probe to core agent.
		// The remote_agent tag is set to "system-probe" via metrics.SetAgentIdentity() above.
		"logs__bytes_sent",

		// Windows Injector metrics (using double underscore format from telemetry component)
		"injector__processes_added_to_injection_tracker",
		"injector__processes_removed_from_injection_tracker",
		"injector__processes_skipped_subsystem",
		"injector__processes_skipped_container",
		"injector__processes_skipped_protected",
		"injector__processes_skipped_system",
		"injector__processes_skipped_excluded",
		"injector__injection_attempts",
		"injector__injection_attempt_failures",
		"injector__injection_max_time_us",
		"injector__injection_successes",
		"injector__injection_failures",
		"injector__pe_caching_failures",
		"injector__import_directory_restoration_failures",
		"injector__pe_memory_allocation_failures",
		"injector__pe_injection_context_allocated",
		"injector__pe_injection_context_cleanedup",
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

// GetStatusDetails returns the status details of system-probe
func (r *remoteagentImpl) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	return helper.DefaultStatusResponse(), nil
}
