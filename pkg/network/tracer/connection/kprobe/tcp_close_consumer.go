// +build linux_bpf

package kprobe

import (
	"sync/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf/manager"
)

type tcpCloseConsumer struct {
	perfHandler  *ddebpf.PerfHandler
	batchManager *perfBatchManager
	requests     chan requestPayload

	closedBuffer  *network.ConnectionBuffer
	maxBufferSize int

	// Telemetry
	perfReceived int64
	perfLost     int64
}

type requestPayload struct {
	buffer       *network.ConnectionBuffer
	responseChan chan struct{}
}

func newTCPCloseConsumer(cfg *config.Config, m *manager.Manager, perfHandler *ddebpf.PerfHandler, filter func(*network.ConnectionStats) bool) (*tcpCloseConsumer, error) {
	connCloseEventMap, _, err := m.GetMap(string(probes.ConnCloseEventMap))
	if err != nil {
		return nil, err
	}
	connCloseMap, _, err := m.GetMap(string(probes.ConnCloseBatchMap))
	if err != nil {
		return nil, err
	}

	numCPUs := int(connCloseEventMap.ABI().MaxEntries)
	batchManager, err := newPerfBatchManager(connCloseMap, numCPUs, filter)
	if err != nil {
		return nil, err
	}

	c := &tcpCloseConsumer{
		perfHandler:   perfHandler,
		batchManager:  batchManager,
		requests:      make(chan requestPayload),
		closedBuffer:  network.NewConnectionBuffer(1024),
		maxBufferSize: cfg.MaxClosedConnectionsBuffered,
	}
	c.start()

	return c, nil
}

func (c *tcpCloseConsumer) GetClosedConnections(buffer *network.ConnectionBuffer) {
	if buffer == nil {
		return
	}

	request := requestPayload{
		buffer:       buffer,
		responseChan: make(chan struct{}),
	}

	c.requests <- request
	<-request.responseChan
}

func (c *tcpCloseConsumer) GetStats() map[string]int64 {
	return map[string]int64{
		"perf_recv": atomic.SwapInt64(&c.perfReceived, 0),
		"perf_lost": atomic.SwapInt64(&c.perfLost, 0),
	}
}

func (c *tcpCloseConsumer) Stop() {
	if c == nil {
		return
	}
	c.perfHandler.Stop()
}

func (c *tcpCloseConsumer) start() {
	go func() {
		for {
			select {
			case batchData, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}

				if c.closedBuffer.Len() >= c.maxBufferSize {
					atomic.AddInt64(&c.perfLost, 1)
					continue
				}

				atomic.AddInt64(&c.perfReceived, 1)
				batch := netebpf.ToBatch(batchData.Data)
				c.batchManager.ExtractBatchInto(c.closedBuffer, batch, batchData.CPU)
			case lostCount, ok := <-c.perfHandler.LostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&c.perfLost, int64(lostCount))
			case request, ok := <-c.requests:
				if !ok {
					return
				}

				c.batchManager.GetPendingConns(c.closedBuffer)
				request.buffer.Append(c.closedBuffer.Connections())
				c.closedBuffer.Reset()
				close(request.responseChan)
			}
		}
	}()
}
