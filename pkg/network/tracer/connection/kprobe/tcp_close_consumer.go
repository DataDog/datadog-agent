// +build linux_bpf

package kprobe

import (
	"sync/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/ebpf/manager"
)

const closedConnectionChanSize = 100

type tcpCloseConsumer struct {
	perfHandler  *ddebpf.PerfHandler
	batchManager *perfBatchManager
	requests     chan requestPayload

	// Telemetry
	perfReceived int64
	perfLost     int64
}

type requestPayload struct {
	buffer       *network.ConnectionBuffer
	responseChan chan struct{}
}

func newTCPCloseConsumer(cfg *config.Config, m *manager.Manager, perfHandler *ddebpf.PerfHandler) (*tcpCloseConsumer, error) {
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
		requests:     make(chan requestPayload),
	}
	return c, nil
}

func (c *tcpCloseConsumer) GetPendingConns() *connection.ClosedBatch {
	if c == nil {
		return nil
	}

	responseBatch := connection.GetBatch()
	request := requestPayload{
		buffer:       responseBatch.Buffer,
		responseChan: make(chan struct{}),
	}

	c.requests <- request
	<-request.responseChan
	return responseBatch
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
	close(c.requests)
}

func (c *tcpCloseConsumer) Start() <-chan *connection.ClosedBatch {
	if c == nil {
		return nil
	}

	out := make(chan *connection.ClosedBatch, closedConnectionChanSize)
	go func() {
		defer close(out)
		for {
			select {
			case batchData, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&c.perfReceived, 1)

				connBatch := connection.GetBatch()
				batch := netebpf.ToBatch(batchData.Data)
				c.batchManager.ExtractBatchInto(connBatch.Buffer, batch, batchData.CPU)
				out <- connBatch
			case lostCount, ok := <-c.perfHandler.LostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&c.perfLost, int64(lostCount))
			case request, ok := <-c.requests:
				if !ok {
					return
				}

				c.batchManager.GetPendingConns(request.buffer)
				close(request.responseChan)
			}
		}
	}()
	return out
}
