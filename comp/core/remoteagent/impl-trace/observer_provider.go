// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package traceimpl

import (
	"context"

	observerbuffer "github.com/DataDog/datadog-agent/comp/trace/observerbuffer/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// GetTraces implements the ObserverProvider gRPC service.
// It drains buffered traces from the observer buffer and returns them to the core-agent.
func (r *remoteagentImpl) GetTraces(ctx context.Context, req *pbcore.GetTracesRequest) (*pbcore.GetTracesResponse, error) {
	if r.observerBuffer == nil {
		return &pbcore.GetTracesResponse{}, nil
	}

	traces, droppedCount, hasMore := r.observerBuffer.DrainTraces(req.GetMaxItems())

	response := &pbcore.GetTracesResponse{
		Traces:       make([]*pbcore.TraceChunkData, 0, len(traces)),
		DroppedCount: droppedCount,
		HasMore:      hasMore,
	}

	for _, t := range traces {
		// Serialize the TracerPayload to msgpack bytes
		payloadData, err := t.Payload.MarshalMsg(nil)
		if err != nil {
			// Skip this trace if serialization fails
			continue
		}
		response.Traces = append(response.Traces, &pbcore.TraceChunkData{
			PayloadData:  payloadData,
			ReceivedAtNs: t.ReceivedAtNs,
		})
	}

	return response, nil
}

// GetProfiles implements the ObserverProvider gRPC service.
// It drains buffered profiles from the observer buffer and returns them to the core-agent.
func (r *remoteagentImpl) GetProfiles(ctx context.Context, req *pbcore.GetProfilesRequest) (*pbcore.GetProfilesResponse, error) {
	if r.observerBuffer == nil {
		return &pbcore.GetProfilesResponse{}, nil
	}

	profiles, droppedCount, hasMore := r.observerBuffer.DrainProfiles(req.GetMaxItems())

	response := &pbcore.GetProfilesResponse{
		Profiles:     make([]*pbcore.ProfileData, 0, len(profiles)),
		DroppedCount: droppedCount,
		HasMore:      hasMore,
	}

	for _, p := range profiles {
		response.Profiles = append(response.Profiles, convertProfileToProto(p))
	}

	return response, nil
}

// convertProfileToProto converts an internal ProfileData to the protobuf message.
func convertProfileToProto(p observerbuffer.ProfileData) *pbcore.ProfileData {
	return &pbcore.ProfileData{
		ProfileId:    p.ProfileID,
		ProfileType:  p.ProfileType,
		Service:      p.Service,
		Env:          p.Env,
		Version:      p.Version,
		Hostname:     p.Hostname,
		ContainerId:  p.ContainerID,
		TimestampNs:  p.TimestampNs,
		DurationNs:   p.DurationNs,
		Tags:         p.Tags,
		ContentType:  p.ContentType,
		InlineData:   p.InlineData,
		ReceivedAtNs: p.ReceivedAtNs,
	}
}
