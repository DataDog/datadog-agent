// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kprobe

import (
	"errors"
	"os"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
	recordPool *sync.Pool
}

// Init the ring buffer
func (rb *RingBuffer) Init(mgr *manager.Manager, config *config.Config) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer("conn_close_event_ring"); !ok {
		return errors.New("couldn't find conn_close_event_ring ring buffer")
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		RecordGetter: func() *ringbuf.Record {
			return rb.recordPool.Get().(*ringbuf.Record)
		},
		DataHandler: rb.handleEvent,
	}

	numCPU, err := utils.NumCPU()
	if err != nil {
		numCPU = 1
	}

	if numCPU <= 16 {
		rb.ringBuffer.RingBufferOptions.RingBufferSize = 8 * 256 * os.Getpagesize()
	}

	rb.ringBuffer.RingBufferOptions.RingBufferSize = 16 * 256 * os.Getpagesize()

	return nil
}

// Start the event stream.
func (rb *RingBuffer) Start() error {
	return rb.ringBuffer.Start()
}

// SetMonitor set the monitor
func (rb *RingBuffer) SetMonitor(counter eventstream.LostEventCounter) {}

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

// New returns a new ring buffer based closed connection stream.
func New(handler func(int, []byte)) *RingBuffer {
	return &RingBuffer{
		handler: handler,
	}
}
