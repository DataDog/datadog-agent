// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ringbuffer holds ringbuffer related files
package ringbuffer

import (
	"context"
	"fmt"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/ringbuf"

	ebpfTelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// defaultDispatcherQueueSize is used when the configured dispatcher queue size is not usable.
const defaultDispatcherQueueSize = 16384

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
	recordPool *ddsync.TypedPool[ringbuf.Record]

	// ctx is owned by the probe and cancelled on shutdown to stop the dispatcher.
	ctx context.Context
	// queue decouples reading the kernel ring buffer from event processing. It
	// acts as a user space cushion to absorb bursts of events.
	queue chan *ringbuf.Record
}

// Init the ring buffer
func (rb *RingBuffer) Init(mgr *manager.Manager, config *config.Config) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer(eventstream.EventStreamMap); !ok {
		return fmt.Errorf("couldn't find %q ring buffer", eventstream.EventStreamMap)
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		RecordGetter: func() *ringbuf.Record {
			return rb.recordPool.Get()
		},
		RecordHandler:    rb.handleEvent,
		TelemetryEnabled: config.InternalTelemetryEnabled,
	}

	queueSize := dispatcherQueueSize(config)
	rb.queue = make(chan *ringbuf.Record, queueSize)
	seclog.Debugf("ring buffer dispatcher queue size set to %d", queueSize)

	ebpfTelemetry.ReportRingBufferTelemetry(rb.ringBuffer)
	return nil
}

// dispatcherQueueSize computes the user space dispatcher queue size from the
// configuration, optionally scaling it by the number of CPUs on the system.
func dispatcherQueueSize(cfg *config.Config) int {
	size := cfg.EventStreamDispatcherQueueSize
	if size <= 0 {
		size = defaultDispatcherQueueSize
	}

	if cfg.EventStreamDispatcherQueueSizePerCore {
		numCPU, err := utils.NumCPU()
		if err != nil || numCPU <= 0 {
			numCPU = 1
		}
		size *= numCPU
	}

	return size
}

// dispatch drains the queue and forwards events to the handler on a dedicated
// goroutine, decoupling event processing from the kernel ring buffer read loop.
// It exits when the probe context is cancelled.
func (rb *RingBuffer) dispatch(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-rb.ctx.Done():
			return
		case record := <-rb.queue:
			rb.handler(0, record.RawSample)
			rb.recordPool.Put(record)
		}
	}
}

// Start the event stream.
func (rb *RingBuffer) Start(wg *sync.WaitGroup) error {
	wg.Add(1)
	go rb.dispatch(wg)
	return rb.ringBuffer.Start()
}

// SetMonitor set the monitor
func (rb *RingBuffer) SetMonitor(_ eventstream.LostEventCounter) {}

func (rb *RingBuffer) handleEvent(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	// Hand the record over to the dispatcher instead of processing it inline, so
	// the kernel ring buffer read loop is never blocked by event processing. The
	// record is returned to the pool by the dispatcher once it has been handled.
	select {
	case rb.queue <- record:
	case <-rb.ctx.Done():
		rb.recordPool.Put(record)
	}
}

// Pause the event stream. Do nothing when using ring buffer
func (rb *RingBuffer) Pause() error {
	return nil
}

// Resume the event stream. Do nothing when using ring buffer
func (rb *RingBuffer) Resume() error {
	return nil
}

// New returns a new ring buffer based event stream.
func New(ctx context.Context, handler func(int, []byte)) *RingBuffer {
	return &RingBuffer{
		ctx:        ctx,
		recordPool: ddsync.NewDefaultTypedPool[ringbuf.Record](),
		handler:    handler,
	}
}
