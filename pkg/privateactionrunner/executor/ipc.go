// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package executor

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// authMetadataKey is the gRPC metadata key carrying the shared agent IPC
// session token. Both processes obtain the token from the IPC component
// (`comp/core/ipc`); the token lives on disk and is loaded identically
// on each side, so no token traverses the command line or environment.
const authMetadataKey = "x-par-executor-token"

// withAuth attaches the executor auth token to outgoing gRPC requests.
func withAuth(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, authMetadataKey, token)
}

// checkAuth rejects incoming requests that lack the expected token. An
// empty expected token disables the check (useful in tests).
func checkAuth(ctx context.Context, expected string) error {
	if expected == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "executor: missing request metadata")
	}
	values := md.Get(authMetadataKey)
	if len(values) == 0 || values[0] != expected {
		return status.Error(codes.Unauthenticated, "executor: invalid auth token")
	}
	return nil
}
