// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package failure contains logic specific to TCP failed connection handling
package failure

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

type failedConnReleaser interface {
	Put(conn *netebpf.FailedConn)
}

// TCPFailedConnConsumer consumes failed connection events from the kernel
type TCPFailedConnConsumer struct {
	eventHandler *perf.EventHandler[netebpf.FailedConn]
	dataChan     <-chan *netebpf.FailedConn
	releaser     failedConnReleaser

	once        sync.Once
	closed      chan struct{}
	FailedConns *FailedConns
}

// NewFailedConnConsumer creates a new TCPFailedConnConsumer
func NewFailedConnConsumer(eventHandler *perf.EventHandler[netebpf.FailedConn], callbackCh <-chan *netebpf.FailedConn, releaser failedConnReleaser, fc *FailedConns) *TCPFailedConnConsumer {
	return &TCPFailedConnConsumer{
		eventHandler: eventHandler,
		releaser:     releaser,
		dataChan:     callbackCh,
		closed:       make(chan struct{}),
		FailedConns:  fc,
	}
}

// Stop stops the consumer
func (c *TCPFailedConnConsumer) Stop() {
	if c == nil {
		return
	}
	//c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
	c.FailedConns.mapCleaner.Stop()
}

// Start starts the consumer
func (c *TCPFailedConnConsumer) Start() {
	if c == nil {
		return
	}

	go func() {
		for {
			select {
			case <-c.closed:
				return
			case failedConn, ok := <-c.dataChan:
				if !ok {
					return
				}
				c.FailedConns.upsertConn(failedConn)
				c.releaser.Put(failedConn)
			}
		}
	}()
}
