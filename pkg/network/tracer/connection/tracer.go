package connection

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/ebpf"
)

// Tracer is the common interface implemented by all connection tracers.
type Tracer interface {
	// Start begins collecting network connection data.
	Start(closedFilter func(*network.ConnectionStats) bool) error
	// Stop halts all network data collection.
	Stop()
	// GetConnections returns the list of currently active connections, using the buffer provided.
	// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
	GetConnections(buffer []network.ConnectionStats, activeFilter func(*network.ConnectionStats) bool) (active, closed []network.ConnectionStats, err error)
	// Remove deletes the connection from tracking state.
	// It does not prevent the connection from re-appearing later, if additional traffic occurs.
	Remove(conn *network.ConnectionStats) error
	// GetTelemetry returns relevant telemetry.
	GetTelemetry() map[string]int64
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) *ebpf.Map
}
