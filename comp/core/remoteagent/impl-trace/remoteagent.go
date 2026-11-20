// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package traceimpl implements the remoteagent component interface
package traceimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"net"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
	Telemetry coretelemetry.Component
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
	// Check if the remoteAgentRegistry is enabled
	if !reqs.Config.GetBool("remote_agent.registry.enabled") {
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

	// Expose the telemetry provider so the core agent can scrape this process's metrics
	// (notably the api_server__request_duration_seconds auth tag, to track mTLS vs token
	// adoption among this agent's IPC clients).
	pbcore.RegisterTelemetryProviderServer(remoteAgentServer.GetGRPCServer(),
		helper.NewTelemetryProviderServer(reqs.Telemetry, coretelemetry.StaticMetricFilter(helper.APIServerRequestDurationMetric)))

	// Expose the flare provider so flares built by the core agent collect this
	// process's expvars through the remote-agent registry, instead of fetching
	// the debug server's unauthenticated /debug/vars endpoint.
	pbcore.RegisterFlareProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)

	remoteAgentServer.Start()

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
	pbcore.UnimplementedFlareProviderServer
}

// GetFlareFiles returns this process's expvars so the core agent can include
// them in flares through the remote-agent registry. The payload matches what
// the debug server's /debug/vars endpoint used to serve (the same expvars,
// YAML-marshaled), so the flare content is equivalent to the HTTP fetch it
// replaces.
func (r *remoteagentImpl) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	variables := make(map[string]interface{})
	expvar.Do(func(kv expvar.KeyValue) {
		var value interface{}
		if err := json.Unmarshal([]byte(kv.Value.String()), &value); err != nil {
			// Keep the raw string so a single bad variable doesn't drop the rest.
			value = kv.Value.String()
		}
		variables[kv.Key] = value
	})

	data, err := yaml.Marshal(variables)
	if err != nil {
		return nil, err
	}

	return &pbcore.GetFlareFilesResponse{
		Files: map[string][]byte{
			"expvar.yaml": data,
		},
	}, nil
}
