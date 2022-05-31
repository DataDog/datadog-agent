// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/pkg/errors"
)

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
}

// Init the ring buffer
func (rb *RingBuffer) Init(mgr *manager.Manager, monitor *Monitor) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer("events"); !ok {
		return errors.New("couldn't find events perf map")
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		DataHandler: rb.handleEvent,
	}

	return nil
}

// Start the event stream.
func (rb *RingBuffer) Start(wg *sync.WaitGroup) error {
	return nil
}

func (rb *RingBuffer) handleEvent(CPU int, data []byte, ringBuffer *manager.RingBuffer, manager *manager.Manager) {
	rb.handler(CPU, data)
}

// Pause the event stream. Do nothing when using ring buffer
func (rb *RingBuffer) Pause() error {
	return nil
}

// Resume the event stream. Do nothing when using ring buffer
func (rb *RingBuffer) Resume() error {
	return nil
}

// NewRingBuffer returns a new ring buffer based event stream.
func NewRingBuffer(handler func(int, []byte)) *RingBuffer {
	return &RingBuffer{
		handler: handler,
	}
}
