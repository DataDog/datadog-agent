//+build linux_bpf

package bytecode

import (
	"github.com/DataDog/ebpf/manager"
)

type ClosedConnPerfHandler struct {
	ClosedChannel chan []byte
	LostChannel   chan uint64
}

func NewClosedConnPerfHandler(closedChannelSize int) *ClosedConnPerfHandler {
	return &ClosedConnPerfHandler{
		ClosedChannel: make(chan []byte, closedChannelSize),
		LostChannel:   make(chan uint64, 10),
	}
}

func (c *ClosedConnPerfHandler) dataHandler(CPU int, batchData []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	c.ClosedChannel <- batchData
}

func (c *ClosedConnPerfHandler) lostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	c.LostChannel <- lostCount
}
