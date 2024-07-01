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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type tcpCloseConsumer struct {
	requests chan chan struct{}
	once     sync.Once
	closed   chan struct{}
	ch       *cookieHasher

	eh       *perf.EventHandler[network.ConnectionStats]
	dataChan <-chan *network.ConnectionStats
	releaser connReleaser
}

type connReleaser interface {
	Put(conn *network.ConnectionStats)
}

func newTCPCloseConsumer(eventHandler *perf.EventHandler[network.ConnectionStats], callbackCh <-chan *network.ConnectionStats, releaser connReleaser) *tcpCloseConsumer {
	return &tcpCloseConsumer{
		requests: make(chan chan struct{}),
		closed:   make(chan struct{}),
		ch:       newCookieHasher(),
		eh:       eventHandler,
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
	//c.eventHandler.Stop()
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

					// TODO log summary?
					//now := time.Now()
					//elapsed := now.Sub(then)
					//then = now
					//log.Debugf(
					//	"tcp close summary: closed_count=%d elapsed=%s closed_rate=%.2f/s lost_samples_count=%d",
					//	closedCount,
					//	elapsed,
					//	float64(closedCount)/elapsed.Seconds(),
					//	lostSamplesCount,
					//)
					//closedCount = 0
					//lostSamplesCount = 0

					continue
				}

				c.ch.Hash(conn)
				callback(conn)
				c.releaser.Put(conn)
			case request := <-c.requests:
				c.eh.Flush()
				flushChannel <- request
			}
		}
	}()
}
