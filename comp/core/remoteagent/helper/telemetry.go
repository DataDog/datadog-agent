// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"context"

	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// APIServerRequestDurationMetric is the prometheus name of the API server request
// duration metric created by comp/api/api/apiimpl/observability. Remote agents expose
// it through the telemetry provider so the core agent can track, per agent flavor,
// whether IPC API clients connect with mTLS or only token authentication.
// Note: the telemetry component joins subsystem and name with a double underscore;
// it is replaced by a '.' when metrics are submitted by the agent telemetry check
// (see api_server.request_duration_seconds in comp/core/agenttelemetry).
const APIServerRequestDurationMetric = "api_server__request_duration_seconds"

// TelemetryProviderServer implements the Remote Agent TelemetryProvider gRPC service by
// forwarding a filtered subset of the process's prometheus metrics to the core agent.
// The core agent's remote agent registry re-exposes forwarded metrics in its own
// telemetry registry with an additional "emitter" label identifying this agent.
type TelemetryProviderServer struct {
	pbcore.UnimplementedTelemetryProviderServer

	telemetry coretelemetry.Component
	filter    coretelemetry.MetricFilter
}

// NewTelemetryProviderServer creates a TelemetryProviderServer forwarding the metric
// families accepted by the given filter.
func NewTelemetryProviderServer(telemetryComp coretelemetry.Component, filter coretelemetry.MetricFilter) *TelemetryProviderServer {
	return &TelemetryProviderServer{
		telemetry: telemetryComp,
		filter:    filter,
	}
}

// GetTelemetry returns the process's telemetry metrics in prometheus text exposition format.
func (t *TelemetryProviderServer) GetTelemetry(context.Context, *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	prometheusText, err := t.telemetry.GatherText(false, t.filter)
	if err != nil {
		return nil, err
	}

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: prometheusText,
		},
	}, nil
}
