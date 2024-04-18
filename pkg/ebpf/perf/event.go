// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package perf implements types related to eBPF and the perf subsystem, like perf buffers and ring buffers.
package perf

import (
	"fmt"
	"slices"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"

	ebpfTelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var perfPool = ddsync.NewDefaultTypedPool[perf.Record]()
var ringbufPool = ddsync.NewDefaultTypedPool[ringbuf.Record]()

// Flushable is an interface for objects that support flushing
type Flushable interface {
	Flush()
}

// EventHandler abstracts consuming data from a perf buffer or ring buffer (depending on availability and options).
// It handles upgrading maps from a ring buffer is desired, and unmarshalling into the desired data type.
// If you wish to use a channel, rather than a callback, see pkg/util/sync.NewCallbackChannel
type EventHandler struct {
	f    Flushable
	opts EventHandlerOptions
}

// EventHandlerOptions are the options controlling the EventHandler.
// MapName and Handler are required options.
type EventHandlerOptions struct {
	MapName string
	Handler func([]byte)

	TelemetryEnabled  bool
	UseRingBuffer     bool
	UpgradePerfBuffer bool

	PerfOptions    PerfBufferOptions
	RingBufOptions RingBufferOptions
}

// PerfBufferOptions are options specifically for perf buffers
//
//nolint:revive
type PerfBufferOptions struct {
	BufferSize int

	// Watermark - The reader will start processing samples once their sizes in the perf ring buffer
	// exceed this value. Must be smaller than PerfRingBufferSize. Defaults to the manager value if not set.
	Watermark int

	// The number of events required in any per CPU buffer before
	// Read will process data. This is mutually exclusive with Watermark.
	// The default is zero, which means Watermark will take precedence.
	WakeupEvents int
}

// RingBufferOptions are options specifically for ring buffers
type RingBufferOptions struct {
	BufferSize int
}

// NewEventHandler creates an event handler with the provided options
func NewEventHandler(opts EventHandlerOptions) (*EventHandler, error) {
	if opts.MapName == "" {
		return nil, fmt.Errorf("invalid options: MapName is required")
	}
	if opts.Handler == nil {
		return nil, fmt.Errorf("invalid options: Handler is required")
	}
	e := &EventHandler{
		opts: opts,
	}
	return e, nil
}

// Init must be called after ebpf-manager.Manager.Load but before ebpf-manager.Manager.Init/InitWithOptions()
func (e *EventHandler) Init(mgr *manager.Manager, mgrOpts *manager.Options) error {
	ms, _, _ := mgr.GetMapSpec(e.opts.MapName)
	if ms == nil {
		return fmt.Errorf("unable to find map spec %q", e.opts.MapName)
	}

	if e.opts.UseRingBuffer && features.HaveMapType(ebpf.RingBuf) == nil {
		if e.opts.UpgradePerfBuffer {
			if ms.Type != ebpf.PerfEventArray {
				return fmt.Errorf("map %q is not a perf buffer, got %q instead", e.opts.MapName, ms.Type.String())
			}
			upgradePerfBuffer(mgr, mgrOpts, e.opts.MapName)
		} else if ms.Type != ebpf.RingBuf {
			return fmt.Errorf("map %q is not a ring buffer, got %q instead", e.opts.MapName, ms.Type.String())
		}

		if ms.MaxEntries != uint32(e.opts.RingBufOptions.BufferSize) {
			resizeRingBuffer(mgrOpts, e.opts.MapName, e.opts.RingBufOptions.BufferSize)
		}
		e.initRingBuffer(mgr)
		return nil
	}

	if ms.Type != ebpf.PerfEventArray {
		return fmt.Errorf("map %q is not a perf buffer, got %q instead", e.opts.MapName, ms.Type.String())
	}
	e.initPerfBuffer(mgr)
	return nil
}

// MapType returns the ebpf.MapType of the underlying events map
// This is only valid after calling Init.
func (e *EventHandler) MapType() ebpf.MapType {
	switch e.f.(type) {
	case *manager.PerfMap:
		return ebpf.PerfEventArray
	case *manager.RingBuffer:
		return ebpf.RingBuf
	default:
		return ebpf.UnspecifiedMap
	}
}

// Flush flushes the pending data from the underlying perfbuf/ringbuf
func (e *EventHandler) Flush() {
	e.f.Flush()
}

func resizeRingBuffer(mgrOpts *manager.Options, mapName string, bufferSize int) {
	if mgrOpts.MapSpecEditors == nil {
		mgrOpts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	specEditor := mgrOpts.MapSpecEditors[mapName]
	specEditor.MaxEntries = uint32(bufferSize)
	specEditor.EditorFlag |= manager.EditMaxEntries
	mgrOpts.MapSpecEditors[mapName] = specEditor
}

func (e *EventHandler) initPerfBuffer(mgr *manager.Manager) {
	mgr.PerfMaps = slices.DeleteFunc(mgr.PerfMaps, func(perfMap *manager.PerfMap) bool {
		return perfMap.Name == e.opts.MapName
	})
	pm := &manager.PerfMap{
		Map: manager.Map{Name: e.opts.MapName},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: e.opts.PerfOptions.BufferSize,
			Watermark:          e.opts.PerfOptions.Watermark,
			WakeupEvents:       e.opts.PerfOptions.WakeupEvents,
			RecordHandler: func(record *perf.Record, perfMap *manager.PerfMap, manager *manager.Manager) {
				defer perfPool.Put(record)
				e.opts.Handler(record.RawSample)
			},
			LostHandler:      nil, // TODO do we need support for Lost?
			RecordGetter:     perfPool.Get,
			TelemetryEnabled: e.opts.TelemetryEnabled,
		},
	}
	mgr.PerfMaps = append(mgr.PerfMaps, pm)
	ebpfTelemetry.ReportPerfMapTelemetry(pm)
	e.f = pm
}

func (e *EventHandler) initRingBuffer(mgr *manager.Manager) {
	mgr.RingBuffers = slices.DeleteFunc(mgr.RingBuffers, func(ringBuf *manager.RingBuffer) bool {
		return ringBuf.Name == e.opts.MapName
	})
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: e.opts.MapName},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler: func(record *ringbuf.Record, ringBuffer *manager.RingBuffer, manager *manager.Manager) {
				defer ringbufPool.Put(record)
				e.opts.Handler(record.RawSample)
			},
			RecordGetter:     ringbufPool.Get,
			TelemetryEnabled: e.opts.TelemetryEnabled,
		},
	}
	mgr.RingBuffers = append(mgr.RingBuffers, rb)
	ebpfTelemetry.ReportRingBufferTelemetry(rb)
	e.f = rb
}

func upgradePerfBuffer(mgr *manager.Manager, mgrOpts *manager.Options, mapName string) {
	if mgrOpts.MapSpecEditors == nil {
		mgrOpts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	mgrOpts.MapSpecEditors[mapName] = manager.MapSpecEditor{
		Type:       ebpf.RingBuf,
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditKeyValue,
	}
	mgr.PerfMaps = slices.DeleteFunc(mgr.PerfMaps, func(perfMap *manager.PerfMap) bool {
		return perfMap.Name == mapName
	})
}
