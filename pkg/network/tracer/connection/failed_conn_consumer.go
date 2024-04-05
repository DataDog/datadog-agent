// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"fmt"
	"sync"
	"time"
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
	eventHandler  ddebpf.EventHandler
	batchManager  *perfBatchManager
	requests      chan chan struct{}
	buffer        *network.ConnectionBuffer
	once          sync.Once
	closed        chan struct{}
	failedConnMap failedConnMap
	mux           sync.Mutex
}

type failedConnStats struct {
	countByErrCode map[uint16]uint16
}

// String returns a string representation of the failedConnStats
func (t failedConnStats) String() string {
	return fmt.Sprintf(
		"failedConnStats{countByErrCode: %v}", t.countByErrCode,
	)
}

type failedConnMap map[netebpf.ConnTuple]*failedConnStats

func newFailedConnConsumer(eventHandler ddebpf.EventHandler, batchManager *perfBatchManager) *tcpFailedConnConsumer {
	return &tcpFailedConnConsumer{
		eventHandler:  eventHandler,
		batchManager:  batchManager,
		requests:      make(chan chan struct{}),
		buffer:        network.NewConnectionBuffer(netebpf.BatchSize, netebpf.BatchSize),
		closed:        make(chan struct{}),
		failedConnMap: make(failedConnMap),
		mux:           sync.Mutex{},
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
	c.mux.Lock()
	defer c.mux.Unlock()
	ct := (*netebpf.FailedConn)(unsafe.Pointer(&data[0]))
	log.Infof("adamk: failed connection: %v, reason: %v", ct.Tup, ct.Reason)

	stats, ok := c.failedConnMap[ct.Tup]
	if !ok {
		stats = &failedConnStats{
			countByErrCode: make(map[uint16]uint16),
		}
		c.failedConnMap[ct.Tup] = stats
	}
	stats.countByErrCode[ct.Reason]++
	// rollup similar conns here
}

func (c *tcpFailedConnConsumer) Start(_ func([]network.ConnectionStats)) {
	if c == nil {
		return
	}

	var (
		then             = time.Now()
		failedCount      uint64
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
					log.Errorf("unknown type received from ring buffer, skipping. data size=%d, expecting %d", len(dataEvent.Data), netebpf.SizeofFailedConn)
					continue
				}

				failedConnConsumerTelemetry.perfReceived.Add(float64(c.buffer.Len()))
				failedCount += uint64(c.buffer.Len())
				c.buffer.Reset()
				dataEvent.Done()
			// lost events only occur when using perf buffers
			case lc, ok := <-lostChannel:
				if !ok {
					return
				}
				failedConnConsumerTelemetry.perfLost.Add(float64(lc))
				lostSamplesCount += lc
			case request := <-c.requests:
				failedCount += uint64(len(c.failedConnMap))
				log.Debugf("adamk failed conn map: %v", c.failedConnMap)
				clear(c.failedConnMap)
				close(request)
				now := time.Now()
				elapsed := now.Sub(then)
				then = now
				log.Debugf(
					"failed conn summary: failed_count=%d elapsed=%s failure_rate=%.2f/s lost_samples_count=%d",
					failedCount,
					elapsed,
					float64(failedCount)/elapsed.Seconds(),
					lostSamplesCount,
				)
				failedCount = 0
				lostSamplesCount = 0
			}
		}
	}()
}
