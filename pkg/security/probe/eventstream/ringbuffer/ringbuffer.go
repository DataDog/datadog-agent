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

const (
	defaultDispatcherQueueSize = 16384
)

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
	recordPool *ddsync.TypedPool[ringbuf.Record]

	ctx        context.Context
	queue      chan *ringbuf.Record // nil when the dispatcher is disabled
	queueBytes atomic.Int64
	occupancy  atomic.Int64
	enqueued   atomic.Uint64
	processed  atomic.Uint64
	peak       atomic.Int64

	statsdClient statsd.ClientInterface
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

	if config.EventStreamDispatcherQueueEnabled {
		queueSize := dispatcherQueueSize(config)
		rb.queue = make(chan *ringbuf.Record, queueSize)
		seclog.Infof("ring buffer dispatcher queue enabled, size=%d", queueSize)
	}

	ebpfTelemetry.ReportRingBufferTelemetry(rb.ringBuffer)
	return nil
}

func dispatcherQueueSize(cfg *config.Config) int {
	return dispatcherQueueSizeWithCPU(cfg, runtime.NumCPU())
}

func dispatcherQueueSizeWithCPU(cfg *config.Config, numCPU int) int {
	size := cfg.EventStreamDispatcherQueueSize
	if size <= 0 {
		size = defaultDispatcherQueueSize
	}

	if cfg.EventStreamDispatcherQueueSizePerCore {
		if numCPU <= 0 {
			numCPU = 1
		}
		size *= numCPU
	}

	if cfg.EventStreamDispatcherQueueSizeMin > size {
		size = cfg.EventStreamDispatcherQueueSizeMin
	}

	return size
}

func (rb *RingBuffer) dispatch(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-rb.ctx.Done():
			rb.drain()
			return
		case record := <-rb.queue:
			rb.handleRecord(record)
		}
	}
}

func (rb *RingBuffer) drain() {
	for {
		select {
		case record := <-rb.queue:
			rb.handleRecord(record)
		default:
			return
		}
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
	rb.processed.Add(1)
	rb.handler(0, record.RawSample)
}

func (rb *RingBuffer) observeDepth(n int) {
	v := int64(n)
	for {
		old := rb.peak.Load()
		if v <= old {
			return
		}
		if rb.peak.CompareAndSwap(old, v) {
			return
		}
	}
}

// SendStats reports dispatcher queue occupancy and counters.
func (rb *RingBuffer) SendStats() error {
	if rb.statsdClient == nil || rb.queue == nil {
		return nil
	}

	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueUsage, float64(rb.occupancy.Load()), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueCapacity, float64(cap(rb.queue)), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueueBytes, float64(rb.queueBytes.Load()), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Gauge(metrics.MetricEventStreamDispatcherQueuePeak, float64(rb.peak.Swap(0)), nil, 1.0); err != nil {
		return err
	}
	if err := rb.statsdClient.Count(metrics.MetricEventStreamDispatcherQueueEnqueued, int64(rb.enqueued.Swap(0)), nil, 1.0); err != nil {
		return err
	}
	return rb.statsdClient.Count(metrics.MetricEventStreamDispatcherQueueProcessed, int64(rb.processed.Swap(0)), nil, 1.0)
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
	if rb.queue == nil {
		rb.handler(0, record.RawSample)
		rb.recordPool.Put(record)
		return
	}

	size := int64(len(record.RawSample))
	rb.queueBytes.Add(size)
	rb.observeDepth(int(rb.occupancy.Add(1)))
	select {
	case rb.queue <- record:
		rb.enqueued.Add(1)
	case <-rb.ctx.Done():
		rb.occupancy.Add(-1)
		rb.queueBytes.Add(-size)
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
func New(ctx context.Context, handler func(int, []byte), statsdClient statsd.ClientInterface) *RingBuffer {
	return &RingBuffer{
		ctx:          ctx,
		recordPool:   ddsync.NewDefaultTypedPool[ringbuf.Record](),
		handler:      handler,
		statsdClient: statsdClient,
	}
}
