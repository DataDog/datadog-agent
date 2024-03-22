// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const failedConnConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var failedConnConsumerTelemetry = struct {
	perfReceived telemetry.Counter
	perfLost     telemetry.Counter
}{
	telemetry.NewCounter(failedConnConsumerModuleName, "failed_conn_polling_received", []string{}, "Counter measuring the number of closed connections received"),
	telemetry.NewCounter(failedConnConsumerModuleName, "failed_conn_polling_lost", []string{}, "Counter measuring the number of closed connection batches lost (were transmitted from ebpf but never received)"),
}

type tcpFailedConnConsumer struct {
	eventHandler ddebpf.EventHandler
	batchManager *perfBatchManager
	requests     chan chan struct{}
	buffer       *network.ConnectionBuffer
	once         sync.Once
	closed       chan struct{}
	ch           *cookieHasher
}

func newFailedConnConsumer(eventHandler ddebpf.EventHandler, batchManager *perfBatchManager) *tcpFailedConnConsumer {
	return &tcpFailedConnConsumer{
		eventHandler: eventHandler,
		batchManager: batchManager,
		requests:     make(chan chan struct{}),
		buffer:       network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
		closed:       make(chan struct{}),
		ch:           newCookieHasher(),
	}
}

func (c *tcpFailedConnConsumer) FlushPending() {
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

func (c *tcpFailedConnConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *tcpFailedConnConsumer) extractConn(data []byte) {
	ct := (*netebpf.FailedConn)(unsafe.Pointer(&data[0]))
	log.Infof("adamk: failed connection: %v", ct)
	// rollup similar conns here
}

func (c *tcpFailedConnConsumer) Start(_ func([]network.ConnectionStats)) {
	if c == nil {
		return
	}

	var (
		//then             = time.Now()
		closedCount      uint64
		lostSamplesCount uint64
	)

	go func() {
		dataChannel := c.eventHandler.DataChannel()
		lostChannel := c.eventHandler.LostChannel()
		for {
			select {

			case <-c.closed:
				return
			case dataEvent, ok := <-dataChannel:
				if !ok {
					return
				}

				l := len(dataEvent.Data)
				switch {
				case l >= netebpf.SizeofFailedConn:
					c.extractConn(dataEvent.Data)
				default:
					log.Errorf("unknown type received from ring buffer, skipping. data size=%d, expecting %d or %d", len(dataEvent.Data), netebpf.SizeofConn, netebpf.SizeofBatch)
					continue
				}

				failedConnConsumerTelemetry.perfReceived.Add(float64(c.buffer.Len()))
				closedCount += uint64(c.buffer.Len())
				c.buffer.Reset()
				dataEvent.Done()
			// lost events only occur when using perf buffers
			case lc, ok := <-lostChannel:
				if !ok {
					return
				}
				failedConnConsumerTelemetry.perfLost.Add(float64(lc))
				lostSamplesCount += lc
				//case request := <-c.requests:
				//	oneTimeBuffer := network.NewConnectionBuffer(32, 32)
				//	c.batchManager.GetPendingConns(oneTimeBuffer)
				//	close(request)
				//
				//	closedCount += uint64(oneTimeBuffer.Len())
				//	now := time.Now()
				//	elapsed := now.Sub(then)
				//	then = now
				//	log.Debugf(
				//		"tcp close summary: closed_count=%d elapsed=%s closed_rate=%.2f/s lost_samples_count=%d",
				//		closedCount,
				//		elapsed,
				//		float64(closedCount)/elapsed.Seconds(),
				//		lostSamplesCount,
				//	)
				//	closedCount = 0
				//	lostSamplesCount = 0
			}
		}
	}()
}
