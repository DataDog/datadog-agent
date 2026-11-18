// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"

	"google.golang.org/protobuf/proto"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
	remotequeriesv1alpha1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/agent_integrations/remote_queries"
)

// remoteQueriesExecuteURI is the published v1alpha1 Execute method. This
// adapter does not handle GetResultChunk.
const remoteQueriesExecuteURI = "rc.x509.magic_tunnel.agent_integrations.remote_queries.v1alpha1.RemoteQueriesService/Execute"

// NewRemoteQueriesExecuteHandler adapts the Remote Queries tunnel to the x509
// client's Execute URI. The tunnel's authority, not this adapter and not the
// request, establishes the org and connection the request must match.
func NewRemoteQueriesExecuteHandler(tunnel *remotequeries.Tunnel, logger log.Component) rcx509client.ExecuteHandler {
	return func(ctx context.Context, correlationID uint64, payload []byte) ([]byte, error) {
		response := executeRemoteQuery(ctx, tunnel, correlationID, payload)
		// Do not log the request or rejection text: both may derive from
		// backend-controlled content. The correlation ID matches the response.
		logger.Infof("remote config x509 remote queries execute handled (correlation_id=%d status=%s code=%q)", correlationID, response.GetStatus(), response.GetErrorCode())
		return proto.Marshal(response)
	}
}

func executeRemoteQuery(ctx context.Context, tunnel *remotequeries.Tunnel, correlationID uint64, payload []byte) *remotequeriesv1alpha1.ExecuteResponse {
	var request remotequeriesv1alpha1.ExecuteRequest
	if err := proto.Unmarshal(payload, &request); err != nil {
		return rejected("invalid_request", "request is invalid")
	}
	outcome := tunnel.Execute(ctx, correlationID, tunnelRequest(&request))
	switch outcome.Status {
	case remotequeries.TunnelAccepted:
		return &remotequeriesv1alpha1.ExecuteResponse{Status: remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ACCEPTED}
	case remotequeries.TunnelAlreadyAccepted:
		return &remotequeriesv1alpha1.ExecuteResponse{Status: remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ALREADY_ACCEPTED}
	case remotequeries.TunnelMatched:
		return &remotequeriesv1alpha1.ExecuteResponse{Status: remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_MATCHED}
	default:
		return rejected(outcome.Code, outcome.Message)
	}
}

func rejected(code, message string) *remotequeriesv1alpha1.ExecuteResponse {
	return &remotequeriesv1alpha1.ExecuteResponse{
		Status:       remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED,
		ErrorCode:    code,
		ErrorMessage: message,
	}
}

func tunnelRequest(request *remotequeriesv1alpha1.ExecuteRequest) remotequeries.TunnelRequest {
	out := remotequeries.TunnelRequest{
		ConnectionID:  request.GetConnectionId(),
		OrgID:         request.GetOrgId(),
		Integration:   request.GetIntegration(),
		Query:         request.GetQuery(),
		IncludeSchema: request.GetIncludeSchema(),
		TaskID:        request.GetTaskId(),
		ResolveOnly:   request.GetResolveOnly(),
		RunID:         request.GetRunId(),
	}
	if target := request.GetTarget(); target != nil {
		out.Target = remotequeries.Target{
			Host:             target.GetHost(),
			Port:             int(target.GetPort()),
			DBName:           target.GetDbname(),
			DatabaseInstance: target.GetDatabaseInstance(),
		}
	}
	if limits := request.GetLimits(); limits != nil {
		out.Limits = &remotequeries.TunnelLimits{
			MaxFileBytes:   limits.GetMaxFileBytes(),
			MaxResultBytes: limits.GetMaxResultBytes(),
			MaxRowBytes:    limits.GetMaxRowBytes(),
			MaxColumns:     limits.GetMaxColumns(),
			MaxSchemaBytes: limits.GetMaxSchemaBytes(),
			MaxPages:       limits.GetMaxPages(),
			TimeoutMs:      limits.GetTimeoutMs(),
		}
	}
	return out
}
