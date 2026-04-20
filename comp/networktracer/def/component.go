// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package networktracer defines the component interface for network connection tracing.
package networktracer

import (
	"context"
	"io"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// team: cloud-network-monitoring

// Component provides network connection tracing capabilities.
// It wraps the underlying eBPF-based tracer on Linux and the driver-based tracer on Windows.
type Component interface {
	// GetActiveConnections returns active network connections for the given clientID.
	// The returned cleanup function MUST be called after the caller is done with the connections.
	GetActiveConnections(clientID string) (*network.Connections, func(), error)

	// RegisterClient registers a new client for connection delta tracking.
	RegisterClient(clientID string) error

	// GetStats returns internal tracer statistics.
	GetStats() map[string]interface{}

	// Pause suspends eBPF-based connection tracking.
	Pause() error

	// Resume resumes eBPF-based connection tracking.
	Resume() error

	// DebugNetworkMaps returns all active connections for debugging purposes.
	// The caller is responsible for calling network.Reclaim on the returned connections.
	DebugNetworkMaps() (*network.Connections, error)

	// DebugNetworkState returns the internal connection state for the given client.
	DebugNetworkState(clientID string) (interface{}, error)

	// DebugEBPFMaps writes the content of all eBPF maps (or a subset identified by maps) to w.
	DebugEBPFMaps(w io.Writer, maps ...string) error

	// DebugCachedConntrack writes the cached conntrack table to w, limited to maxSize entries.
	DebugCachedConntrack(ctx context.Context, w io.Writer, maxSize int) error

	// DebugHostConntrack writes the host conntrack table to w, limited to maxSize entries.
	DebugHostConntrack(ctx context.Context, w io.Writer, maxSize int) error

	// DebugDumpProcessCache returns the content of the process cache.
	DebugDumpProcessCache(ctx context.Context) (interface{}, error)

	// IsSupported reports whether the tracer is operational on the current platform and kernel.
	// Returns false on platforms where eBPF is unavailable or the kernel version is unsupported.
	IsSupported() bool
}
