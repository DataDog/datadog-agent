// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"sync"
	"time"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const closeConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var closerConsumerTelemetry = struct {
	perfReceived telemetry.Counter
	perfLost     telemetry.Counter
}{
	telemetry.NewCounter(closeConsumerModuleName, "closed_conn_polling_received", []string{}, "Counter measuring the number of closed connections received"),
	telemetry.NewCounter(closeConsumerModuleName, "closed_conn_polling_lost", []string{}, "Counter measuring the number of connections lost (were transmitted from ebpf but never received)"),
}

type tcpCloseConsumer struct {
	perfHandler  *ddebpf.PerfHandler
	batchManager *perfBatchManager
	requests     chan chan struct{}
	buffer       *network.ConnectionBuffer
	once         sync.Once
	closed       chan struct{}
	ch           *cookieHasher
}

func newTCPCloseConsumer(perfHandler *ddebpf.PerfHandler, batchManager *perfBatchManager) *tcpCloseConsumer {
	return &tcpCloseConsumer{
		perfHandler:  perfHandler,
		batchManager: batchManager,
		requests:     make(chan chan struct{}),
		buffer:       network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
		closed:       make(chan struct{}),
		ch:           newCookieHasher(),
	}
}

func (c *tcpCloseConsumer) FlushPending() {
	if c == nil {
		return
	}

	select {
	case <-c.closed:
		return
	default:
	}

	wait := make(chan struct{})
	select {
	case <-c.closed:
	case c.requests <- wait:
		<-wait
	}
}

func (c *tcpCloseConsumer) Stop() {
	if c == nil {
		return
	}
	c.perfHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *tcpCloseConsumer) extractConn(data []byte) {
	ct := (*netebpf.Conn)(unsafe.Pointer(&data[0]))
	conn := c.buffer.Next()
	populateConnStats(conn, &ct.Tup, &ct.Conn_stats, c.ch)
	updateTCPStats(conn, ct.Conn_stats.Cookie, &ct.Tcp_stats)
}

func (c *tcpCloseConsumer) Start(callback func([]network.ConnectionStats)) {
	if c == nil {
		return
	}

	var (
		then        time.Time = time.Now()
		closedCount uint64
		lostCount   uint64
	)

	go func() {
		for {
			select {
			case <-c.closed:
				return
			case batchData, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}

				l := len(batchData.Data)
				switch {
				case l >= netebpf.SizeofBatch:
					batch := netebpf.ToBatch(batchData.Data)
					c.batchManager.ExtractBatchInto(c.buffer, batch, batchData.CPU)
				case l >= netebpf.SizeofConn:
					c.extractConn(batchData.Data)
				default:
					log.Errorf("unknown type received from perf buffer, skipping. data size=%d, expecting %d or %d", len(batchData.Data), netebpf.SizeofConn, netebpf.SizeofBatch)
					continue
				}

				closerConsumerTelemetry.perfReceived.Add(float64(c.buffer.Len()))
				closedCount += uint64(c.buffer.Len())
				callback(c.buffer.Connections())
				c.buffer.Reset()
				batchData.Done()
			case lc, ok := <-c.perfHandler.LostChannel:
				if !ok {
					return
				}
				closerConsumerTelemetry.perfLost.Add(float64(lc * netebpf.BatchSize))
				lostCount += lc * netebpf.BatchSize
			case request := <-c.requests:
				oneTimeBuffer := network.NewConnectionBuffer(32, 32)
				c.batchManager.GetPendingConns(oneTimeBuffer)
				callback(oneTimeBuffer.Connections())
				close(request)

				closedCount += uint64(oneTimeBuffer.Len())
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
