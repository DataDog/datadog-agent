// +build !linux_bpf linux_bpf,!bcc

package ebpf

import "github.com/DataDog/datadog-agent/pkg/ebpf/tcpqueuelength"

// TCPQueueLengthTracer is not implemented on non-linux systems
type TCPQueueLengthTracer struct{}

// NewTCPQueueLengthTracer is not implemented on non-linux systems
func NewTCPQueueLengthTracer() (*TCPQueueLengthTracer, error) {
	return nil, ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) Close() {}

// Get is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) Get() []tcpqueuelength.Stats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *TCPQueueLengthTracer) GetAndFlush() []tcpqueuelength.Stats {
	return nil
}
