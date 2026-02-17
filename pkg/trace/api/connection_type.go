// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import "context"

// ConnectionType represents the type of network transport used for a connection.
type ConnectionType string

const (
	// ConnectionTypeTCP indicates a TCP connection.
	ConnectionTypeTCP ConnectionType = "tcp"
	// ConnectionTypeUDS indicates a Unix Domain Socket connection.
	ConnectionTypeUDS ConnectionType = "uds"
	// ConnectionTypePipe indicates a Windows named pipe connection.
	ConnectionTypePipe ConnectionType = "pipe"
	// ConnectionTypeUnknown indicates an unknown connection type (should not happen).
	ConnectionTypeUnknown ConnectionType = "unknown"
)

type connectionTypeKey struct{}

// withConnectionType stores the connection type in the context.
func withConnectionType(ctx context.Context, ct ConnectionType) context.Context {
	return context.WithValue(ctx, connectionTypeKey{}, ct)
}

// GetConnectionType retrieves the connection type from the context.
// Returns an empty string if not set.
func GetConnectionType(ctx context.Context) ConnectionType {
	if ct, ok := ctx.Value(connectionTypeKey{}).(ConnectionType); ok {
		return ct
	}
	return ""
}
