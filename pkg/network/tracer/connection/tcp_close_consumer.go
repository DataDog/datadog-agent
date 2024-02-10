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
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const closeConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var closeConsumerTelemetry = struct {
	perfReceived telemetry.Counter
	perfLost     telemetry.Counter
}{
	telemetry.NewCounter(closeConsumerModuleName, "closed_conn_polling_received", []string{}, "Counter measuring the number of closed connections received"),
	telemetry.NewCounter(closeConsumerModuleName, "closed_conn_polling_lost", []string{}, "Counter measuring the number of closed connection batches lost (were transmitted from ebpf but never received)"),
}

type tcpCloseConsumer struct {
	eventHandler ddebpf.EventHandler
	batchManager *perfBatchManager
	requests     chan chan struct{}
	buffer       *network.ConnectionBuffer
	once         sync.Once
	closed       chan struct{}
	ch           *cookieHasher
}

func newTCPCloseConsumer(eventHandler ddebpf.EventHandler, batchManager *perfBatchManager) *tcpCloseConsumer {
	return &tcpCloseConsumer{
		eventHandler: eventHandler,
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
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *tcpCloseConsumer) extractConn(data []byte) {
	ct := (*netebpf.Conn)(unsafe.Pointer(&data[0]))
	conn := c.buffer.Next()
	populateConnStats(conn, &ct.Tup, &ct.Conn_stats, c.ch)
	updateTCPStats(conn, &ct.Tcp_stats, ct.Tcp_retransmits)
}

func (c *tcpCloseConsumer) Start(callback func([]network.ConnectionStats)) {
	if c == nil {
		return
	}
	health := health.RegisterLiveness("network-tracer")

	var (
		then             = time.Now()
		closedCount      uint64
		lostSamplesCount uint64
	)

	go func() {
		defer func() {
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
		}()

		dataChannel := c.eventHandler.DataChannel()
		lostChannel := c.eventHandler.LostChannel()
		for {
			select {

			case <-c.closed:
				return
			case <-health.C:
			case batchData, ok := <-dataChannel:
				log.Debugf("adamk received data from perf/ring buffer")
				if !ok {
					return
				}

				l := len(batchData.Data)
				switch {
				case l >= netebpf.SizeofBatch:
					log.Debugf("adamk received batch data from perf/ring buffer")
					batch := netebpf.ToBatch(batchData.Data)
					c.batchManager.ExtractBatchInto(c.buffer, batch)
				case l >= netebpf.SizeofConn:
					log.Debugf("adamk received conn data from perf/ring buffer")
					c.extractConn(batchData.Data)
				default:
					log.Errorf("unknown type received from perf buffer, skipping. data size=%d, expecting %d or %d", len(batchData.Data), netebpf.SizeofConn, netebpf.SizeofBatch)
					continue
				}

				closeConsumerTelemetry.perfReceived.Add(float64(c.buffer.Len()))
				closedCount += uint64(c.buffer.Len())
				callback(c.buffer.Connections())
				c.buffer.Reset()
				batchData.Done()
			// lost events only happen when using perf buffers
			case lc, ok := <-lostChannel:
				if !ok {
					return
				}
				closeConsumerTelemetry.perfLost.Add(float64(lc))
				lostSamplesCount += lc
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
					"tcp close summary: closed_count=%d elapsed=%s closed_rate=%.2f/s lost_samples_count=%d",
					closedCount,
					elapsed,
					float64(closedCount)/elapsed.Seconds(),
					lostSamplesCount,
				)
				closedCount = 0
				lostSamplesCount = 0
			}
		}
	}()
}
