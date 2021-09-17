package connection

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/ebpf"
)

// Tracer is the common interface implemented by all connection tracers.
type Tracer interface {
	// Start begins collecting network connection data.
	// It returns a channel which contains closed connections as they arrive
	Start() (<-chan network.ConnectionStats, error)
	// Stop halts all network data collection.
	Stop()
	// GetConnections returns the list of currently active connections, using the buffer provided.
	// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
	GetConnections(buffer []network.ConnectionStats, filter func(*network.ConnectionStats) bool) ([]network.ConnectionStats, error)
	// FlushPending forces any closed connections waiting for batching to be returned immediately.
	// This allows synchronous processing to occur, rather than waiting an unknown amount of time for closed connections
	// to appear on the channel returned from Start.
	FlushPending() []network.ConnectionStats
	// Remove deletes the connection from tracking state.
	// It does not prevent the connection from re-appearing later, if additional traffic occurs.
	Remove(conn *network.ConnectionStats) error
	// GetTelemetry returns relevant telemetry.
	GetTelemetry() map[string]int64
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) *ebpf.Map
}
