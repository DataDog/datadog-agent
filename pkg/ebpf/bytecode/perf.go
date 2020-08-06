//+build linux_bpf

package bytecode

import (
	"sync"

	"github.com/DataDog/ebpf/manager"
)

type PerfHandler struct {
	ClosedChannel chan []byte
	LostChannel   chan uint64
	once          sync.Once
	closed        bool
}

func NewPerfHandler(closedChannelSize int) *PerfHandler {
	return &PerfHandler{
		ClosedChannel: make(chan []byte, closedChannelSize),
		LostChannel:   make(chan uint64, 10),
	}
}

func (c *PerfHandler) dataHandler(CPU int, batchData []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.ClosedChannel <- batchData
}

func (c *PerfHandler) lostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.LostChannel <- lostCount
}

func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.ClosedChannel)
		close(c.LostChannel)
	})
}
