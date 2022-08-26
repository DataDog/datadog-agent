// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

type failedConnStats struct {
	failureCount uint64
}

type failedConnMap map[netebpf.ConnTuple]failedConnStats

type tcpFailedConnConsumer struct {
	perfHandler *ddebpf.PerfHandler
	failedConns failedConnMap

	mutex sync.Mutex
}

func newTCPFailedConnConsumer(perfHandler *ddebpf.PerfHandler) (*tcpFailedConnConsumer, error) {
	return &tcpFailedConnConsumer{
		perfHandler: perfHandler,
		failedConns: make(failedConnMap),
	}, nil
}

func (c *tcpFailedConnConsumer) Start() {
	if c == nil {
		return
	}

	go func() {
		for {
			select {
			case rawConnTuple, ok := <-c.perfHandler.DataChannel:
				if !ok {
					return
				}

				connTuple := toConnTuple(rawConnTuple.Data)
				c.addFailedConn(connTuple)
			}
		}
	}()
}

func (c *tcpFailedConnConsumer) Stop() {
	if c == nil {
		return
	}

	c.perfHandler.Stop()
}

// Returns the latest conn data
func (c *tcpFailedConnConsumer) GetStats() failedConnMap {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	res := c.failedConns
	c.failedConns = make(failedConnMap)

	return res
}

// Utils

func (c *tcpFailedConnConsumer) addFailedConn(t *netebpf.ConnTuple) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	stats := c.failedConns[*t]
	stats.failureCount += 1

	c.failedConns[*t] = stats
}

func toConnTuple(data []byte) *netebpf.ConnTuple {
	return (*netebpf.ConnTuple)(unsafe.Pointer(&data[0]))
}
