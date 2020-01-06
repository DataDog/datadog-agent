// +build !linux_bpf

package ebpf

type TCPQueueLengthTracer struct{}

func NewTCPQueueLengthTracer() (*TCPQueueLengthTracer, error) {
	return nil, ErrNotImplemented
}

func (t *TCPQueueLengthTracer) Close() {}

func (t *TCPQueueLengthTracer) Get() []Stats {
	return nil
}

func (t *TCPQueueLengthTracer) GetAndFlush() []Stats {
	return nil
}
