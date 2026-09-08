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
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/ringbuf"

	ebpfTelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
	recordPool *ddsync.TypedPool[ringbuf.Record]

	ctx        context.Context
	queue      *byteQueue // nil when the dispatcher is disabled
	queueBytes atomic.Int64
	occupancy  atomic.Int64
	enqueued   atomic.Uint64

	statsdClient statsd.ClientInterface
}

// Init the ring buffer
func (rb *RingBuffer) Init(mgr *manager.Manager, config *config.Config) error {
	var ok bool
	if rb.ringBuffer, ok = mgr.GetRingBuffer(eventstream.EventStreamMap); !ok {
		return fmt.Errorf("couldn't find %q ring buffer", eventstream.EventStreamMap)
	}

	recordHandler := rb.handleEvent
	if config.EventStreamDispatcherQueueEnabled {
		maxBytes := dispatcherQueueMaxBytes(config)
		rb.queue = newByteQueue(maxBytes)
		recordHandler = rb.handleQueuedEvent
		seclog.Infof("ring buffer dispatcher queue enabled, max_bytes=%d", maxBytes)
	}

	rb.ringBuffer.RingBufferOptions = manager.RingBufferOptions{
		RecordGetter: func() *ringbuf.Record {
			return rb.recordPool.Get()
		},
		RecordHandler:    recordHandler,
		TelemetryEnabled: config.InternalTelemetryEnabled,
	}

	ebpfTelemetry.ReportRingBufferTelemetry(rb.ringBuffer)
	return nil
}

func dispatcherQueueMaxBytes(cfg *config.Config) int64 {
	return dispatcherQueueMaxBytesWithCPU(cfg, runtime.NumCPU())
}

func dispatcherQueueMaxBytesWithCPU(cfg *config.Config, numCPU int) int64 {
	size := int64(cfg.EventStreamDispatcherQueueSize)

	if cfg.EventStreamDispatcherQueueSizePerCore {
		if numCPU <= 0 {
			numCPU = 1
		}
		size *= int64(numCPU)
	}

	if minBytes := int64(cfg.EventStreamDispatcherQueueSizeMin); minBytes > size {
		size = minBytes
	}

	if size < 0 {
		size = 0
	}

	return size
}

func (rb *RingBuffer) dispatch(wg *sync.WaitGroup) {
	defer wg.Done()

	stop := context.AfterFunc(rb.ctx, rb.queue.close)
	defer stop()

	for {
		record, ok := rb.queue.dequeue()
		if !ok {
			return
		}
		rb.handleRecord(record)
	}
}

func (rb *RingBuffer) handleRecord(record *ringbuf.Record) {
	defer func() {
		if r := recover(); r != nil {
			seclog.Errorf("ring buffer dispatcher panic: %v", r)
		}
		rb.recordPool.Put(record)
	}()

	rb.occupancy.Add(-1)
	rb.queueBytes.Add(-int64(len(record.RawSample)))
	rb.handler(0, record.RawSample)
}

// SendStats reports dispatcher queue occupancy and counters.
func (rb *RingBuffer) SendStats() error {
	if rb.statsdClient == nil || rb.queue == nil {
		return nil
	}

	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueUsage, float64(rb.occupancy.Load()), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueCapacity, float64(rb.queue.capacity()), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueBytes, float64(rb.queueBytes.Load()), nil, 1.0); err != nil {
		return err
	}
	return rb.statsdClient.Count(metrics.MetricEventStreamDispatcherQueueEnqueued, int64(rb.enqueued.Swap(0)), nil, 1.0)
}

// Start the event stream.
func (rb *RingBuffer) Start(wg *sync.WaitGroup) error {
	if err := rb.ringBuffer.Start(); err != nil {
		return err
	}

	if rb.queue == nil {
		return nil
	}

	wg.Add(1)
	go rb.dispatch(wg)

	return nil
}

// SetMonitor set the monitor
func (rb *RingBuffer) SetMonitor(_ eventstream.LostEventCounter) {}

func (rb *RingBuffer) handleEvent(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	rb.handler(0, record.RawSample)
	rb.recordPool.Put(record)
}

func (rb *RingBuffer) handleQueuedEvent(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	if !rb.queue.enqueueAndPublish(record, func(size int64) {
		rb.enqueued.Add(1)
		rb.occupancy.Add(1)
		rb.queueBytes.Add(size)
	}) {
		rb.recordPool.Put(record)
		return
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
func New(ctx context.Context, handler func(int, []byte), statsdClient statsd.ClientInterface) *RingBuffer {
	return &RingBuffer{
		ctx:          ctx,
		recordPool:   ddsync.NewDefaultTypedPool[ringbuf.Record](),
		handler:      handler,
		statsdClient: statsdClient,
	}
}
