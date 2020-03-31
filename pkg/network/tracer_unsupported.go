// +build !linux_bpf,!windows

package network

import "github.com/DataDog/datadog-agent/pkg/ebpf"

// Tracer is not implemented
type Tracer struct{}

// NewTracer is not implemented on this OS for Tracer
func NewTracer(_ *Config) (*Tracer, error) {
	return nil, ebpf.ErrNotImplemented
}

// Stop is not implemented on this OS for Tracer
func (t *Tracer) Stop() {}

// GetActiveConnections is not implemented on this OS for Tracer
func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetStats is not implemented on this OS for Tracer
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugState is not implemented on this OS for Tracer
func (t *Tracer) DebugState(clientID string) (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugNetworkMaps is not implemented on this OS for Tracer
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ebpf.ErrNotImplemented
}
