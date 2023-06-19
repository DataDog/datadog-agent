// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ringbuffer

import (
	"errors"
	"sync"
	"sync/atomic"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
)

type RingBufferHandler struct {
	cb func(int, []byte)
}

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer     *manager.RingBuffer
	eventHandler   RingBufferHandler
	discardHandler RingBufferHandler
	currentHandler atomic.Pointer[RingBufferHandler]
}

// Init the ring buffer
func (rb *RingBuffer) Init(mgr *manager.Manager, config *config.Config) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer("events"); !ok {
		return errors.New("couldn't find events ring buffer")
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		DataHandler: rb.handleEvent,
	}

	if config.EventStreamBufferSize != 0 {
		rb.ringBuffer.RingBufferOptions.RingBufferSize = config.EventStreamBufferSize
	}

	return nil
}

// Start the event stream.
func (rb *RingBuffer) Start(wg *sync.WaitGroup) error {
	rb.currentHandler.Store(&rb.eventHandler)
	return nil
}

// SetMonitor set the monitor
func (rb *RingBuffer) SetMonitor(counter eventstream.LostEventCounter) {}

func (rb *RingBuffer) handleEvent(CPU int, data []byte, ringBuffer *manager.RingBuffer, manager *manager.Manager) {
	rb.currentHandler.Load().cb(CPU, data)
}

// Pause the event stream. New events will be lost.
func (rb *RingBuffer) Pause() error {
	rb.currentHandler.Store(&rb.discardHandler)
	return nil
}

// Resume the event stream.
func (rb *RingBuffer) Resume() error {
	rb.currentHandler.Store(&rb.eventHandler)
	return nil
}

// New returns a new ring buffer based event stream.
func New(handler func(int, []byte)) *RingBuffer {
	rb := &RingBuffer{
		eventHandler: RingBufferHandler{
			cb: handler,
		},
		discardHandler: RingBufferHandler{
			cb: func(i int, b []byte) {},
		},
	}
	rb.currentHandler.Store(&rb.discardHandler)

	return rb
}
