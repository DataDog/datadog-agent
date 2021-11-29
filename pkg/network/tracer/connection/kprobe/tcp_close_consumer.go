// +build linux_bpf

package kprobe

import (
	"sync"
	"sync/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf/manager"
)

const (
	perfReceivedStat = "perf_recv"
	perfLostStat     = "perf_lost"
)

type tcpCloseConsumer struct {
	perfHandler  *ddebpf.PerfHandler
	batchManager *perfBatchManager
	requests     chan chan struct{}
	buffer       *network.ConnectionBuffer
	once         sync.Once

	// Telemetry
	perfReceived int64
	perfLost     int64
}

func newTCPCloseConsumer(m *manager.Manager, perfHandler *ddebpf.PerfHandler) (*tcpCloseConsumer, error) {
	connCloseEventMap, _, err := m.GetMap(string(probes.ConnCloseEventMap))
	if err != nil {
		return nil, err
	}
	connCloseMap, _, err := m.GetMap(string(probes.ConnCloseBatchMap))
	if err != nil {
		return nil, err
	}

	numCPUs := int(connCloseEventMap.ABI().MaxEntries)
	batchManager, err := newPerfBatchManager(connCloseMap, numCPUs)
	if err != nil {
		return nil, err
	}

	c := &tcpCloseConsumer{
		perfHandler:  perfHandler,
		batchManager: batchManager,
		requests:     make(chan chan struct{}),
		buffer:       network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
	}
	return c, nil
}

func (c *tcpCloseConsumer) FlushPending() {
	if c == nil {
		return
	}

	wait := make(chan struct{})
	c.requests <- wait
	<-wait
}

func (c *tcpCloseConsumer) GetStats() map[string]int64 {
	return map[string]int64{
		perfReceivedStat: atomic.SwapInt64(&c.perfReceived, 0),
		perfLostStat:     atomic.SwapInt64(&c.perfLost, 0),
	}
}

func (c *tcpCloseConsumer) Stop() {
	if c == nil {
		return
	}
	c.perfHandler.Stop()
	c.once.Do(func() {
		close(c.requests)
	})
}

func (c *tcpCloseConsumer) Start(callback func([]network.ConnectionStats)) {
	if c == nil {
		return
	}

	go func() {
		for {
			select {
			case batchData, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&c.perfReceived, 1)
				batch := netebpf.ToBatch(batchData.Data)
				c.batchManager.ExtractBatchInto(c.buffer, batch, batchData.CPU)
				callback(c.buffer.Connections())
				c.buffer.Reset()
			case lostCount, ok := <-c.perfHandler.LostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&c.perfLost, int64(lostCount))
			case request, ok := <-c.requests:
				if !ok {
					return
				}

				oneTimeBuffer := network.NewConnectionBuffer(32, 32)
				c.batchManager.GetPendingConns(oneTimeBuffer)
				callback(oneTimeBuffer.Connections())
				close(request)
			}
		}
	}()
}
