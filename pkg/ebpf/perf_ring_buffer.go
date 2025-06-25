// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"
	"sync/atomic"

	"github.com/cilium/ebpf/ringbuf"

	manager "github.com/DataDog/ebpf-manager"

	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var ringPool = ddsync.NewDefaultTypedPool[ringbuf.Record]()

// RingBufferHandler implements EventHandler
// this line is just a static check of the interface
var _ EventHandler = new(RingBufferHandler)

// RingBufferHandler wraps an eBPF ring buffer
type RingBufferHandler struct {
	RecordGetter func() *ringbuf.Record

	dataChannel chan DataEvent
	lostChannel chan uint64
	once        sync.Once
	closed      bool

	chLenTelemetry *atomic.Uint64
}

// NewRingBufferHandler creates a RingBufferHandler
func NewRingBufferHandler(dataChannelSize int) *RingBufferHandler {
	return &RingBufferHandler{
		RecordGetter: func() *ringbuf.Record {
			return ringPool.Get()
		},
		dataChannel: make(chan DataEvent, dataChannelSize),
		// This channel is not really used in the context of ring buffers but
		// it's here so `RingBufferHandler` and `PerfHandler` can be used
		// interchangeably
		lostChannel:    make(chan uint64, 1),
		chLenTelemetry: &atomic.Uint64{},
	}
}

// RecordHandler is the callback intended to be used when configuring PerfMapOptions
func (c *RingBufferHandler) RecordHandler(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	if c.closed {
		return
	}

	c.dataChannel <- DataEvent{Data: record.RawSample, rr: record}
	updateMaxTelemetry(c.chLenTelemetry, uint64(len(c.dataChannel)))
}

// DataChannel returns the channel with event data
func (c *RingBufferHandler) DataChannel() <-chan DataEvent {
	return c.dataChannel
}

// LostChannel returns the channel with lost events
func (c *RingBufferHandler) LostChannel() <-chan uint64 {
	return c.lostChannel
}

// GetChannelLengthTelemetry returns the channel length telemetry
func (c *RingBufferHandler) GetChannelLengthTelemetry() *atomic.Uint64 { return c.chLenTelemetry }

// Stop stops the perf handler and closes both channels
func (c *RingBufferHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.dataChannel)
		close(c.lostChannel)
	})
}

// implement the CAS algorithm to atomically update a max value
func updateMaxTelemetry(a *atomic.Uint64, val uint64) {
	for {
		oldVal := a.Load()
		if val <= oldVal {
			return
		}
		// if the value at a is not `oldVal`, then `CompareAndSwap` returns
		// false indicating that the value of the atomic has changed between
		// the above check and this invocation.
		// In this case we retry the above test, to see if the value still needs
		// to be updated.
		if a.CompareAndSwap(oldVal, val) {
			return
		}
	}
}
