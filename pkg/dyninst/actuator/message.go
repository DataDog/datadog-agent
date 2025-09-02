// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"sync"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// Message is an item read from the ringbuffer.
type Message struct {
	rec *ringbuf.Record
}

var recordPool = sync.Pool{
	New: func() any {
		return new(ringbuf.Record)
	},
}

// Event returns the event corresponding to the message.
//
// The caller should not access the event or its data anymore after calling
// Release.
func (m Message) Event() output.Event {
	return output.Event(m.rec.RawSample)
}

// Release returns the message to the pool.
func (m *Message) Release() {
	recordPool.Put(m.rec)
	m.rec = nil
}
