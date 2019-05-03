// +build !linux_bpf

package ebpf

// CurrentKernelVersion is not implemented on non-linux systems
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}

// Tracer is not implemented on non-linux systems
type Tracer struct{}

// NewTracer is not implemented on non-linux systems
func NewTracer(_ *Config) (*Tracer, error) {
	return nil, ErrNotImplemented
}

// Stop is not implemented on non-linux systems
func (t *Tracer) Stop() {}

// GetActiveConnections is not implemented on non-linux systems
func (t *Tracer) GetActiveConnections(_ string) (*Connections, error) {
	return nil, ErrNotImplemented
}

// GetStats is not implemented on non-linux systems
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkState is not implemented on non-linux systems
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// DebugNetworkMaps is not implemented on non-linux systems
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	return nil, ErrNotImplemented
}
