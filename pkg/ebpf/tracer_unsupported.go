// +build !linux_bpf,!windows

package ebpf

import "github.com/DataDog/datadog-agent/pkg/network"

// Tracer is not implemented
type Tracer struct{}

// NewTracer is not implemented on this OS for Tracer
func NewTracer(_ *Config) (*Tracer, error) {
	return nil, ErrNotImplemented
}

// Stop is not implemented on this OS for Tracer
func (t *Tracer) Stop() {}

// GetActiveConnections is not implemented on this OS for Tracer
func (t *Tracer) GetActiveConnections(_ string) (*network.Connections, error) {
	return nil, ErrNotImplemented
}

// GetStats is not implemented on this OS for Tracer
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkState is not implemented on this OS for Tracer
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkMaps is not implemented on this OS for Tracer
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, ErrNotImplemented
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}
