// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package failure contains logic specific to TCP failed connection handling
package failure

import (
	"sync"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const failedConnConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var failedConnConsumerTelemetry = struct {
	eventsReceived telemetry.Counter
	eventsLost     telemetry.Counter
}{
	telemetry.NewCounter(failedConnConsumerModuleName, "failed_conn_polling_received", []string{}, "Counter measuring the number of failed connections received"),
	telemetry.NewCounter(failedConnConsumerModuleName, "failed_conn_polling_lost", []string{}, "Counter measuring the number of failed connections lost (were transmitted from ebpf but never received)"),
}

// TCPFailedConnConsumer consumes failed connection events from the kernel
type TCPFailedConnConsumer struct {
	eventHandler ddebpf.EventHandler
	once         sync.Once
	closed       chan struct{}
	FailedConns  *FailedConns
}

// NewFailedConnConsumer creates a new TCPFailedConnConsumer
func NewFailedConnConsumer(eventHandler ddebpf.EventHandler, m *manager.Manager, maxFailedConnsBuffered uint32) *TCPFailedConnConsumer {
	return &TCPFailedConnConsumer{
		eventHandler: eventHandler,
		closed:       make(chan struct{}),
		FailedConns:  NewFailedConns(m, maxFailedConnsBuffered),
	}
}

// Stop stops the consumer
func (c *TCPFailedConnConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
	c.FailedConns.connCloseFlushedCleaner.Stop()
}

func (c *TCPFailedConnConsumer) extractConn(data []byte) {
	failedConn := (*Conn)(unsafe.Pointer(&data[0]))
	failedConnConsumerTelemetry.eventsReceived.Inc()

	c.FailedConns.UpsertConn(failedConn)
}

// Start starts the consumer
func (c *TCPFailedConnConsumer) Start() {
	if c == nil {
		return
	}

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
				failedConnConsumerTelemetry.eventsLost.Inc()
				dataEvent.Done()
			// lost events only occur when using perf buffers
			case lc, ok := <-lostChannel:
				if !ok {
					return
				}
				failedConnConsumerTelemetry.eventsLost.Add(float64(lc))
			}
		}
	}()
}
