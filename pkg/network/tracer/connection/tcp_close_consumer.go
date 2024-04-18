// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const closeConsumerModuleName = "network_tracer__ebpf"

// Telemetry
var closeConsumerTelemetry = struct {
	perfReceived telemetry.Counter
}{
	telemetry.NewCounter(closeConsumerModuleName, "closed_conn_polling_received", []string{}, "Counter measuring the number of closed connections received"),
}

type tcpCloseConsumer struct {
	requests chan chan struct{}
	once     sync.Once
	closed   chan struct{}

	flusher  perf.Flushable
	dataChan <-chan *network.ConnectionStats
	releaser ddsync.PoolReleaser[network.ConnectionStats]
}

func newTCPCloseConsumer(flusher perf.Flushable, callbackCh <-chan *network.ConnectionStats, releaser ddsync.PoolReleaser[network.ConnectionStats]) *tcpCloseConsumer {
	return &tcpCloseConsumer{
		requests: make(chan chan struct{}),
		closed:   make(chan struct{}),
		flusher:  flusher,
		dataChan: callbackCh,
		releaser: releaser,
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
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *tcpCloseConsumer) Start(callback func(*network.ConnectionStats)) {
	if c == nil {
		return
	}
	liveHealth := health.RegisterLiveness("network-tracer")

	go func() {
		defer func() {
			err := liveHealth.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
		}()

		flushChannel := make(chan chan struct{}, 1)
		for {
			select {

			case <-c.closed:
				return
			case <-liveHealth.C:
			case conn, ok := <-c.dataChan:
				if !ok {
					return
				}
				// sentinel record post-flush
				if conn == nil {
					request := <-flushChannel
					close(request)
					continue
				}

				closeConsumerTelemetry.perfReceived.Inc()
				callback(conn)
				c.releaser.Put(conn)
			case request := <-c.requests:
				c.flusher.Flush()
				flushChannel <- request
			}
		}
	}()
}
