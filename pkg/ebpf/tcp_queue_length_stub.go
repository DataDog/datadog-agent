// +build !linux_bpf

package ebpf

// TCPQueueLengthTracer is not implemented on non-linux systems
type TCPQueueLengthTracer struct{}

// NewTCPQUeueLengthTracer is not implemented on non-linux systems
func NewTCPQueueLengthTracer() (*TCPQueueLengthTracer, error) {
	return nil, ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) Close() {}

// Get is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) Get() []Stats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) GetAndFlush() []Stats {
	return nil
}
