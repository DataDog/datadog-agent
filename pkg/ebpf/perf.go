// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"

	"github.com/cilium/ebpf/perf"

	manager "github.com/DataDog/ebpf-manager"
)

var recordPool = sync.Pool{
	New: func() interface{} {
		return new(perf.Record)
	},
}

// PerfHandler implements EventHandler
// this line is just a static check of the interface
var _ EventHandler = new(PerfHandler)

// PerfHandler wraps an eBPF perf buffer
type PerfHandler struct {
	RecordGetter func() *perf.Record

	dataChannel chan *DataEvent
	lostChannel chan uint64
	once        sync.Once
	closed      bool
}

// NewPerfHandler creates a PerfHandler
func NewPerfHandler(dataChannelSize int) *PerfHandler {
	return &PerfHandler{
		RecordGetter: func() *perf.Record {
			return recordPool.Get().(*perf.Record)
		},
		dataChannel: make(chan *DataEvent, dataChannelSize),
		lostChannel: make(chan uint64, 10),
	}
}

// LostHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) LostHandler(_ int, lostCount uint64, _ *manager.PerfMap, _ *manager.Manager) {
	if c.closed {
		return
	}
	c.lostChannel <- lostCount
}

// RecordHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) RecordHandler(record *perf.Record, _ *manager.PerfMap, _ *manager.Manager) {
	if c.closed {
		return
	}

	c.dataChannel <- &DataEvent{Data: record.RawSample, pr: record}
}

// DataChannel returns the channel with event data
func (c *PerfHandler) DataChannel() <-chan *DataEvent {
	return c.dataChannel
}

// LostChannel returns the channel with lost events
func (c *PerfHandler) LostChannel() <-chan uint64 {
	return c.lostChannel
}

// Stop stops the perf handler and closes both channels
func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.dataChannel)
		close(c.lostChannel)
	})
}
