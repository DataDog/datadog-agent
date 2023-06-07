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

// PerfHandler wraps an eBPF perf buffer
type PerfHandler struct {
	DataChannel  chan *DataEvent
	LostChannel  chan uint64
	RecordGetter func() *perf.Record

	once   sync.Once
	data   chan *DataEvent
	lost   chan uint64
	closed chan struct{}
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
	pf := &PerfHandler{
		DataChannel: make(chan *DataEvent, dataChannelSize),
		LostChannel: make(chan uint64, 10),
		RecordGetter: func() *perf.Record {
			return recordPool.Get().(*perf.Record)
		},
		data:   make(chan *DataEvent, 1),
		lost:   make(chan uint64, 1),
		closed: make(chan struct{}),
	}

	go func() {
		defer func() {
			close(pf.DataChannel)
		}()

		var d *DataEvent
		for {
			if d != nil {
				select {
				case <-pf.closed:
					return
				case pf.DataChannel <- d:
				}
			}

			select {
			case <-pf.closed:
				return
			case d = <-pf.data:
			}
		}
	}()

	go func() {
		defer func() {
			close(pf.LostChannel)
		}()

		var l uint64
		for {
			if l > 0 {
				select {
				case <-pf.closed:
					return
				case pf.LostChannel <- l:
				}
			}

			select {
			case <-pf.closed:
				return
			case l = <-pf.lost:
			}
		}
	}()

	return pf
}

// LostHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) LostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	select {
	case <-c.closed:
	case c.lost <- lostCount:
	}
}

// RecordHandler is the callback intended to be used when configuring PerfMapOptions
func (c *PerfHandler) RecordHandler(record *perf.Record, perfMap *manager.PerfMap, manager *manager.Manager) {
	select {
	case <-c.closed:
	case c.data <- &DataEvent{CPU: record.CPU, Data: record.RawSample, r: record}:
	}
}

// Stop stops the perf handler and closes both channels
func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		close(c.closed)
	})
}
