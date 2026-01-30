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
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/ringbuf"
	"golang.org/x/sys/unix"

	ebpfTelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

type Event = []byte

// RingBuffer implements the EventStream interface
// using an eBPF map of type BPF_MAP_TYPE_RINGBUF
type RingBuffer struct {
	ringBuffer *manager.RingBuffer
	handler    func(int, []byte)
	recordPool *ddsync.TypedPool[ringbuf.Record]
	EventQueue chan Event
	ctx        context.Context
	cancelFunc context.CancelFunc
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

	rb.EventQueue = make(chan Event, 100000)
	ebpfTelemetry.ReportRingBufferTelemetry(rb.ringBuffer)
	return nil
}

func (rb *RingBuffer) startDispatcherLoop(ctx context.Context) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		runtime.LockOSThread()
		if err := unix.Setpriority(unix.PRIO_PROCESS, unix.Gettid(), -20); err != nil {
			seclog.Warnf("failed to renice kernel event ingestion thread (ringbuf) to nice=-18: %v", err)
		}

		for {
			select {
			case <-signalChan:
				return
			case event := <-rb.EventQueue:
				rb.handler(0, event)
			}
		}
	}()

}

// Start the event stream.
func (rb *RingBuffer) Start(_ *sync.WaitGroup) error {
	ctx, _ := context.WithCancel(context.Background())
	rb.startDispatcherLoop(ctx)
	err := rb.ringBuffer.Start()
	//cancel()
	return err
}

// SetMonitor set the monitor
func (rb *RingBuffer) SetMonitor(_ eventstream.LostEventCounter) {}

func (rb *RingBuffer) handleEvent(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	dataCopy := make([]byte, len(record.RawSample))
	copy(dataCopy, record.RawSample)
	rb.EventQueue <- dataCopy
	rb.recordPool.Put(record)
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
func New(handler func(int, []byte)) *RingBuffer {
	return &RingBuffer{
		recordPool: ddsync.NewDefaultTypedPool[ringbuf.Record](),
		handler:    handler,
	}
}
