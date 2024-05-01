// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package failed

import (
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
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

// TcpFailedConnConsumer consumes failed connection events from the kernel
type TcpFailedConnConsumer struct {
	eventHandler ddebpf.EventHandler
	once         sync.Once
	closed       chan struct{}
	FailedConns  *FailedConns
}

// NewFailedConnConsumer creates a new TcpFailedConnConsumer
func NewFailedConnConsumer(eventHandler ddebpf.EventHandler) *TcpFailedConnConsumer {
	return &TcpFailedConnConsumer{
		eventHandler: eventHandler,
		closed:       make(chan struct{}),
		FailedConns:  NewFailedConns(),
	}
}

func (c *TcpFailedConnConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *TcpFailedConnConsumer) extractConn(data []byte) {
	c.FailedConns.Lock()
	defer c.FailedConns.Unlock()
	ct := (*netebpf.FailedConn)(unsafe.Pointer(&data[0]))

	stats, ok := c.FailedConns.FailedConnMap[ct.Tup]
	if !ok {
		stats = &FailedConnStats{
			CountByErrCode: make(map[uint32]uint32),
		}
		c.FailedConns.FailedConnMap[ct.Tup] = stats
	}
	stats.CountByErrCode[ct.Reason]++
}

// Start starts the consumer
func (c *TcpFailedConnConsumer) Start() {
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
