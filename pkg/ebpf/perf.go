// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/perf"
)

// PerfHandler wraps an eBPF perf buffer
type PerfHandler struct {
	DataChannel  chan *DataEvent
	LostChannel  chan uint64
	RecordGetter func() *perf.Record
	once         sync.Once
	closed       bool
}

// DataEvent is a single event read from a perf buffer
type DataEvent struct {
	CPU  int
	Data []byte

	r *perf.Record
}

// Done returns the data buffer back to a sync.Pool
func (d *DataEvent) Done() {
	recordPool.Put(d.r)
}

var recordPool = sync.Pool{
	New: func() interface{} {
		return &perf.Record{}
	},
}

// NewPerfHandler creates a PerfHandler
func NewPerfHandler(dataChannelSize int) *PerfHandler {
	return &PerfHandler{
		DataChannel: make(chan *DataEvent, dataChannelSize),
		LostChannel: make(chan uint64, 10),
		RecordGetter: func() *perf.Record {
			return recordPool.Get().(*perf.Record)
		},
	}
}

// LostHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) LostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.LostChannel <- lostCount
}

// RecordHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) RecordHandler(record *perf.Record, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}

	c.DataChannel <- &DataEvent{CPU: record.CPU, Data: record.RawSample, r: record}
}

// Stop stops the perf handler and closes both channels
func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.DataChannel)
		close(c.LostChannel)
	})
}
