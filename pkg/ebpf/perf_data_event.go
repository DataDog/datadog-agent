// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
)

// EventHandler is the common interface shared across perf map and perf ring
// buffer handlers
type EventHandler interface {
	DataChannel() <-chan *DataEvent
	LostChannel() <-chan uint64
	Stop()
}

// DataEvent encapsulates a single event read from a perf buffer
type DataEvent struct {
	Data []byte

	pr *perf.Record
	rr *ringbuf.Record
}

// Done returns the data buffer back to a sync.Pool
func (d *DataEvent) Done() {
	if d.pr != nil {
		recordPool.Put(d.pr)
		return
	}

	if d.rr != nil {
		ringPool.Put(d.rr)
	}
}
