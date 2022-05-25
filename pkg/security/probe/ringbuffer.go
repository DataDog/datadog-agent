package probe

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/pkg/errors"
)

type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
}

func (rb *RingBuffer) Init(mgr *manager.Manager, monitor *Monitor) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer("events"); !ok {
		return errors.New("couldn't find events perf map")
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		DataHandler: rb.handleEvent,
	}

	return nil
}

func (rb *RingBuffer) Start(wg *sync.WaitGroup) error {
	return nil
}

func (rb *RingBuffer) handleEvent(CPU int, data []byte, ringBuffer *manager.RingBuffer, manager *manager.Manager) {
	rb.handler(CPU, data)
}

func (rb *RingBuffer) Pause() error {
	return nil
}

func (rb *RingBuffer) Resume() error {
	return nil
}

func NewRingBuffer(handler func(int, []byte)) *RingBuffer {
	return &RingBuffer{
		handler: handler,
	}
}
