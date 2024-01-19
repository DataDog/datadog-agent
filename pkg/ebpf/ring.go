// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"

	"github.com/cilium/ebpf/ringbuf"

	manager "github.com/DataDog/ebpf-manager"
)

// RingHandler wraps an eBPF ring buffer
type RingHandler struct {
	DataChannel  chan *RingDataEvent
	LostChannel  chan uint64
	RecordGetter func() *ringbuf.Record
	once         sync.Once
	closed       bool
}

// RingDataEvent is a single event read from a perf buffer
type RingDataEvent struct {
	Data []byte

	r *ringbuf.Record
}

// Done returns the data buffer back to a sync.Pool
func (d *RingDataEvent) Done() {
	eventPool.Put(d.r)
}

var eventPool = sync.Pool{
	New: func() interface{} {
		return &ringbuf.Record{}
	},
}

// NewRingHandler creates a RingHandler
func NewRingHandler(dataChannelSize int) *RingHandler {
	return &RingHandler{
		DataChannel: make(chan *RingDataEvent, dataChannelSize),
		LostChannel: make(chan uint64, 10),
		RecordGetter: func() *ringbuf.Record {
			return eventPool.Get().(*ringbuf.Record)
		},
	}
}

// LostHandler is the callback intended to be used when configuring RingMapOptions
func (c *RingHandler) LostHandler(_ int, lostCount uint64, _ *manager.RingBuffer, _ *manager.Manager) {
	if c.closed {
		return
	}
	c.LostChannel <- lostCount
}

// RecordHandler is the callback intended to be used when configuring RingMapOptions
func (c *RingHandler) RecordHandler(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	if c.closed {
		return
	}

	c.DataChannel <- &RingDataEvent{Data: record.RawSample, r: record}
}

// Stop stops the perf handler and closes both channels
func (c *RingHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.DataChannel)
		close(c.LostChannel)
	})
}
