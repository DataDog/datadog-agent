// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connection

import (
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// Tracer is the common interface implemented by all connection tracers.
type Tracer interface {
	// Start begins collecting network connection data.
	Start(func([]network.ConnectionStats)) error
	// Stop halts all network data collection.
	Stop()
	// GetConnections returns the list of currently active connections, using the buffer provided.
	// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
	GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error
	// FlushPending forces any closed connections waiting for batching to be processed immediately.
	FlushPending()
	// Remove deletes the connection from tracking state.
	// It does not prevent the connection from re-appearing later, if additional traffic occurs.
	Remove(conn *network.ConnectionStats) error
	// GetTelemetry returns relevant telemetry.
	GetTelemetry() map[string]int64
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) *ebpf.Map
	// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
	DumpMaps(maps ...string) (string, error)
}
