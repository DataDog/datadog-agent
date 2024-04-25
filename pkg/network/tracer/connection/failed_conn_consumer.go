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
	eventHandler  ddebpf.EventHandler
	requests      chan chan struct{}
	once          sync.Once
	closed        chan struct{}
	failedConnMap *network.FailedConns
	mux           sync.Mutex
}

func newFailedConnConsumer(eventHandler ddebpf.EventHandler) *tcpFailedConnConsumer {
	return &tcpFailedConnConsumer{
		eventHandler:  eventHandler,
		requests:      make(chan chan struct{}),
		closed:        make(chan struct{}),
		failedConnMap: network.NewFailedConns(),
		mux:           sync.Mutex{},
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
	c.failedConnMap.Lock()
	defer c.failedConnMap.Unlock()
	ct := (*netebpf.FailedConn)(unsafe.Pointer(&data[0]))

	stats, ok := c.failedConnMap.FailedConnMap[ct.Tup]
	if !ok {
		stats = &network.FailedConnStats{
			CountByErrCode: make(map[uint32]uint32),
		}
		c.failedConnMap.FailedConnMap[ct.Tup] = stats
	}
	stats.CountByErrCode[ct.Reason]++
}

func (c *tcpFailedConnConsumer) Start() {
	if c == nil {
		return
	}

	var (
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
					log.Errorf("unknown type received from buffer, skipping. data size=%d, expecting %d", len(dataEvent.Data), netebpf.SizeofFailedConn)
					continue
				}
				failedConnConsumerTelemetry.perfLost.Inc()
				dataEvent.Done()
			// lost events only occur when using perf buffers
			case lc, ok := <-lostChannel:
				if !ok {
					return
				}
				failedConnConsumerTelemetry.perfLost.Add(float64(lc))
				lostSamplesCount += lc
			}
		}
	}()
}
