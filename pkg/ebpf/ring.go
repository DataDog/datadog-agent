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

var ringPool = sync.Pool{
	New: func() interface{} {
		return new(ringbuf.Record)
	},
}

var _ EventHandler = new(RingBufferHandler)

// RingBufferHandler wraps an eBPF ring buffer
type RingBufferHandler struct {
	RecordGetter func() *ringbuf.Record

	dataChannel chan *DataEvent
	lostChannel chan uint64
	once        sync.Once
	closed      bool
}

// NewRingBufferHandler creates a RingBufferHandler
func NewRingBufferHandler(dataChannelSize int) *RingBufferHandler {
	return &RingBufferHandler{
		RecordGetter: func() *ringbuf.Record {
			return ringPool.Get().(*ringbuf.Record)
		},
		dataChannel: make(chan *DataEvent, dataChannelSize),
		// This channel is not used in the context of ring buffers, but
		// it's here so `RingBufferHandler` and `PerfHandler` can be used
		// interchangeably
		lostChannel: make(chan uint64, 1),
	}
}

// RecordHandler is the callback intended to be used when configuring PerfMapOptions
func (c *RingBufferHandler) RecordHandler(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	if c.closed {
		return
	}

	c.dataChannel <- &DataEvent{Data: record.RawSample, rr: record}
}

// DataChannel returns the channel with event data
func (c *RingBufferHandler) DataChannel() <-chan *DataEvent {
	return c.dataChannel
}

// LostChannel returns the channel with lost events
func (c *RingBufferHandler) LostChannel() <-chan uint64 {
	return c.lostChannel
}

// Stop stops the perf handler and closes both channels
func (c *RingBufferHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.dataChannel)
		close(c.lostChannel)
	})
}
