// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package connection

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
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
	perfReceived *atomic.Int64
	perfLost     *atomic.Int64
}

func newTCPCloseConsumer(m *manager.Manager, perfHandler *ddebpf.PerfHandler, batchManager *perfBatchManager) (*tcpCloseConsumer, error) {
	c := &tcpCloseConsumer{
		perfHandler:  perfHandler,
		batchManager: batchManager,
		requests:     make(chan chan struct{}),
		buffer:       network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
		perfReceived: atomic.NewInt64(0),
		perfLost:     atomic.NewInt64(0),
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
		perfReceivedStat: c.perfReceived.Load(),
		perfLostStat:     c.perfLost.Load(),
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

	var (
		then        time.Time = time.Now()
		closedCount int
		lostCount   int
	)
	go func() {
		for {
			select {
			case batchData, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}

				c.perfReceived.Inc()
				batch := netebpf.ToBatch(batchData.Data)
				c.batchManager.ExtractBatchInto(c.buffer, batch, batchData.CPU)
				closedCount += c.buffer.Len()
				callback(c.buffer.Connections())
				c.buffer.Reset()
				batchData.Done()
			case lc, ok := <-c.perfHandler.LostChannel:
				if !ok {
					return
				}
				c.perfLost.Add(int64(lc))
				lostCount += netebpf.BatchSize
			case request, ok := <-c.requests:
				if !ok {
					return
				}

				oneTimeBuffer := network.NewConnectionBuffer(32, 32)
				c.batchManager.GetPendingConns(oneTimeBuffer)
				callback(oneTimeBuffer.Connections())
				close(request)

				closedCount += oneTimeBuffer.Len()
				now := time.Now()
				elapsed := now.Sub(then)
				then = now
				log.Debugf(
					"tcp close summary: closed_count=%d elapsed=%s closed_rate=%.2f/s lost_count=%d",
					closedCount,
					elapsed,
					float64(closedCount)/elapsed.Seconds(),
					lostCount,
				)
				closedCount = 0
				lostCount = 0
			}
		}
	}()
}
