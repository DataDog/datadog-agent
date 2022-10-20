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

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	PerfReceivedStat = "perf_recv"
	PerfLostStat     = "perf_lost"
)

type TCPCloseConsumer struct {
	perfHandler  *ddebpf.PerfHandler
	batchManager *PerfBatchManager
	requests     chan chan struct{}
	buffer       *network.ConnectionBuffer
	once         sync.Once

	// Telemetry
	perfReceived *atomic.Int64
	perfLost     *atomic.Int64
}

func NewTCPCloseConsumer(m *manager.Manager, perfHandler *ddebpf.PerfHandler, batchManager *PerfBatchManager) (*TCPCloseConsumer, error) {
	c := &TCPCloseConsumer{
		perfHandler:  perfHandler,
		batchManager: batchManager,
		requests:     make(chan chan struct{}),
		buffer:       network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
		perfReceived: atomic.NewInt64(0),
		perfLost:     atomic.NewInt64(0),
	}
	return c, nil
}

func (c *TCPCloseConsumer) FlushPending() {
	if c == nil {
		return
	}

	wait := make(chan struct{})
	c.requests <- wait
	<-wait
}

func (c *TCPCloseConsumer) GetStats() map[string]int64 {
	return map[string]int64{
		PerfReceivedStat: c.perfReceived.Load(),
		PerfLostStat:     c.perfLost.Load(),
	}
}

func (c *TCPCloseConsumer) Stop() {
	if c == nil {
		return
	}
	c.perfHandler.Stop()
	c.once.Do(func() {
		close(c.requests)
	})
}

func (c *TCPCloseConsumer) Start(callback func([]network.ConnectionStats)) {
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
