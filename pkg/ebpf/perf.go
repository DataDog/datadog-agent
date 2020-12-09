//+build linux_bpf

package ebpf

import (
	"sync"

	"github.com/DataDog/ebpf/manager"
)

type PerfHandler struct {
	DataChannel chan []byte
	LostChannel chan uint64
	once        sync.Once
	closed      bool
}

func NewPerfHandler(dataChannelSize int) *PerfHandler {
	return &PerfHandler{
		DataChannel: make(chan []byte, dataChannelSize),
		LostChannel: make(chan uint64, 10),
	}
}

func (c *PerfHandler) DataHandler(CPU int, data []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.DataChannel <- data
}

func (c *PerfHandler) LostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.LostChannel <- lostCount
}

func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.DataChannel)
		close(c.LostChannel)
	})
}
