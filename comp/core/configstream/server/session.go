// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

// ValidateSessionID extracts and validates the remote-agent session from gRPC metadata.
// As a side effect it calls RefreshRemoteAgent, extending the session heartbeat so that
// any RAR-gated RPC also acts as a liveness ping.
func ValidateSessionID(ctx context.Context, registry remoteagentregistry.Component, action string) (string, error) {
	if registry == nil {
		return "", status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}

	sessionIDs := md.Get("session_id")
	if len(sessionIDs) == 0 {
		return "", status.Errorf(codes.Unauthenticated, "session_id required in metadata: remote agent must register with RAR before %s", action)
	}

	sessionID := sessionIDs[0]
	if sessionID == "" {
		return "", status.Errorf(codes.Unauthenticated, "session_id cannot be empty: remote agent must register with RAR before %s", action)
	}

	if !registry.RefreshRemoteAgent(sessionID) {
		return "", status.Errorf(codes.PermissionDenied, "session_id '%s' not found: remote agent must register with RAR before %s", sessionID, action)
	}

	return sessionID, nil
}
