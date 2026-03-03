// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

// Package perf implements types related to eBPF and the perf subsystem, like perf buffers and ring buffers.
package perf

import (
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	ebpfTelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var perfPool = ddsync.NewDefaultTypedPool[perf.Record]()
var ringbufPool = ddsync.NewDefaultTypedPool[ringbuf.Record]()

// Flusher is an interface for objects that support flushing
type Flusher interface {
	Flush()
}

// compile time check to ensure this satisfies the Modifier* interfaces
var _ ddebpf.ModifierPreStart = (*EventHandler)(nil)
var _ ddebpf.ModifierAfterStop = (*EventHandler)(nil)

// EventHandler abstracts consuming data from a perf buffer or ring buffer (depending on availability and options).
// It handles upgrading maps from a ring buffer if desired, and unmarshalling into the desired data type.
type EventHandler struct {
	f    Flusher
	opts eventHandlerOptions
	// mapName specifies the name of the map
	mapName string
	// handler is the callback for data received from the perf/ring buffer
	handler func([]byte)

	readLoop func()
	perfChan chan *perf.Record
	ringChan chan *ringbuf.Record

	chLenTelemetry *atomic.Uint64
}

type mapMode uint8

const (
	perfBufferOnly mapMode = iota
	upgradePerfBuffer
	ringBufferOnly
)

// EventHandlerMode controls the mode in which the event handler operates
type EventHandlerMode func(*EventHandler)

// UsePerfBuffers will only use perf buffers and will not attempt any upgrades to ring buffers.
func UsePerfBuffers(bufferSize int, channelSize int, perfMode PerfBufferMode) EventHandlerMode {
	return func(e *EventHandler) {
		e.opts.mode = perfBufferOnly
		e.opts.channelSize = channelSize
		e.opts.perfBufferSize = bufferSize
		perfMode(&e.opts.perfOptions)
	}
}

// UpgradePerfBuffers will upgrade to ring buffers if available, but will fall back to perf buffers if not.
func UpgradePerfBuffers(perfBufferSize int, channelSize int, perfMode PerfBufferMode, ringBufferSize int) EventHandlerMode {
	return func(e *EventHandler) {
		e.opts.mode = upgradePerfBuffer
		e.opts.channelSize = channelSize
		e.opts.perfBufferSize = perfBufferSize
		e.opts.ringBufferSize = ringBufferSize
		perfMode(&e.opts.perfOptions)
	}
}

// UseRingBuffers will only use ring buffers.
func UseRingBuffers(bufferSize int, channelSize int) EventHandlerMode {
	return func(e *EventHandler) {
		e.opts.mode = ringBufferOnly
		e.opts.channelSize = channelSize
		e.opts.ringBufferSize = bufferSize
	}
}

// EventHandlerOption is an option that applies to the event handler
type EventHandlerOption func(*EventHandler)

// SendTelemetry specifies whether to collect usage telemetry from the perf/ring buffer
func SendTelemetry(enabled bool) EventHandlerOption {
	return func(e *EventHandler) {
		e.opts.telemetryEnabled = enabled
	}
}

// RingBufferEnabledConstantName provides a constant name that will be set whether ring buffers are in use
func RingBufferEnabledConstantName(name string) EventHandlerOption {
	return func(e *EventHandler) {
		e.opts.ringBufferEnabledConstantName = name
	}
}

// RingBufferWakeupSize sets a constant for eBPF to use, that determines when to wakeup userspace
func RingBufferWakeupSize(name string, size uint64) EventHandlerOption {
	return func(e *EventHandler) {
		e.opts.ringBufferWakeupConstantName = name
		e.opts.ringBufferWakeupSize = size
	}
}

// eventHandlerOptions are the options controlling the EventHandler.
type eventHandlerOptions struct {
	// telemetryEnabled specifies whether to collect usage telemetry from the perf/ring buffer.
	telemetryEnabled bool

	mode        mapMode
	channelSize int

	perfBufferSize int
	perfOptions    perfBufferOptions

	ringBufferSize                int
	ringBufferEnabledConstantName string

	ringBufferWakeupConstantName string
	ringBufferWakeupSize         uint64
}

// PerfBufferMode is a mode for the perf buffer
//
//nolint:revive
type PerfBufferMode func(*perfBufferOptions)

// Watermark - The reader will start processing samples once their sizes in the perf buffer
// exceed this value. Must be smaller than the perf buffer size.
func Watermark(byteCount int) PerfBufferMode {
	return func(opts *perfBufferOptions) {
		opts.watermark = byteCount
		opts.wakeupEvents = 0
	}
}

// WakeupEvents - The number of events required in any per CPU buffer before Read will process data.
func WakeupEvents(count int) PerfBufferMode {
	return func(opts *perfBufferOptions) {
		opts.wakeupEvents = count
		opts.watermark = 0
	}
}

// perfBufferOptions are options specifically for perf buffers
//
//nolint:revive
type perfBufferOptions struct {
	watermark    int
	wakeupEvents int
}

// NewEventHandler creates an event handler with the provided options
func NewEventHandler(mapName string, handler func([]byte), mode EventHandlerMode, opts ...EventHandlerOption) (*EventHandler, error) {
	if mapName == "" {
		return nil, errors.New("invalid options: MapName is required")
	}
	if handler == nil {
		return nil, errors.New("invalid options: Handler is required")
	}
	e := &EventHandler{
		mapName: mapName,
		handler: handler,
	}
	mode(e)
	for _, opt := range opts {
		opt(e)
	}
	if e.opts.telemetryEnabled {
		e.chLenTelemetry = &atomic.Uint64{}
	}
	return e, nil
}

// BeforeInit implements the Modifier interface
// This function will modify the shared buffers according to the user provided mode
func (e *EventHandler) BeforeInit(mgr *manager.Manager, moduleName names.ModuleName, mgrOpts *manager.Options) (err error) {
	ms, _, _ := mgr.GetMapSpec(e.mapName)
	if ms == nil {
		return fmt.Errorf("unable to find map spec %q", e.mapName)
	}
	defer e.setupEnabledConstant(mgrOpts)
	defer e.setupRingbufferWakeupConstant(mgrOpts)

	ringBufErr := features.HaveMapType(ebpf.RingBuf)
	if e.opts.mode == ringBufferOnly {
		if ringBufErr != nil {
			return ringBufErr
		}
		if ms.Type != ebpf.RingBuf {
			return fmt.Errorf("map %q is not a ring buffer, got %q instead", e.mapName, ms.Type.String())
		}

		// the size of the ring buffer is communicated to the kernel via the max entries field
		// of the bpf map
		if ms.MaxEntries != uint32(e.opts.ringBufferSize) {
			ResizeRingBuffer(mgrOpts, e.mapName, e.opts.ringBufferSize)
		}
		e.initRingBuffer(mgr)
		return nil
	}
	defer e.removeRingBufferHelperCalls(mgr, moduleName, mgrOpts)

	if e.opts.mode == perfBufferOnly {
		if ms.Type != ebpf.PerfEventArray {
			return fmt.Errorf("map %q is not a perf buffer, got %q instead", e.mapName, ms.Type.String())
		}
		e.initPerfBuffer(mgr)
		return nil
	}

	if e.opts.mode == upgradePerfBuffer {
		if ms.Type != ebpf.PerfEventArray {
			return fmt.Errorf("map %q is not a perf buffer, got %q instead", e.mapName, ms.Type.String())
		}

		// the layout of the bpf map for perf buffers does not match that of ring buffers.
		// When upgrading perf buffers to ring buffers, we must account for these differences.
		// - Ring buffers do not use key/value sizes
		// - Ring buffers specify their size via max entries
		if ringBufErr == nil {
			UpgradePerfBuffer(mgr, mgrOpts, e.mapName)
			if ms.MaxEntries != uint32(e.opts.ringBufferSize) {
				ResizeRingBuffer(mgrOpts, e.mapName, e.opts.ringBufferSize)
			}
			e.initRingBuffer(mgr)
			return nil
		}

		e.initPerfBuffer(mgr)
		return nil
	}

	return fmt.Errorf("unsupported EventHandlerMode %d", e.opts.mode)
}

func (e *EventHandler) removeRingBufferHelperCalls(mgr *manager.Manager, moduleName names.ModuleName, mgrOpts *manager.Options) {
	if features.HaveMapType(ebpf.RingBuf) == nil {
		return
	}
	// add helper call remover because ring buffers are not available
	_ = ddebpf.NewHelperCallRemover(asm.FnRingbufOutput, asm.FnRingbufQuery, asm.FnRingbufReserve, asm.FnRingbufSubmit, asm.FnRingbufDiscard).BeforeInit(mgr, moduleName, mgrOpts)
}

func (e *EventHandler) setupEnabledConstant(mgrOpts *manager.Options) {
	if e.opts.ringBufferEnabledConstantName == "" || e.f == nil {
		return
	}

	var val uint64
	switch e.f.(type) {
	case *manager.RingBuffer:
		val = uint64(1)
	default:
		val = uint64(0)
	}
	mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, manager.ConstantEditor{
		Name:  e.opts.ringBufferEnabledConstantName,
		Value: val,
	})
}

func (e *EventHandler) setupRingbufferWakeupConstant(mgrOpts *manager.Options) {
	if e.opts.ringBufferWakeupSize == 0 || e.opts.ringBufferWakeupConstantName == "" || e.f == nil {
		return
	}

	switch e.f.(type) {
	case *manager.RingBuffer:
		mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, manager.ConstantEditor{
			Name:  e.opts.ringBufferWakeupConstantName,
			Value: e.opts.ringBufferWakeupSize,
		})
	default:
		// do nothing
	}
}

// PreStart implements the ModifierPreStart interface
func (e *EventHandler) PreStart(_ *manager.Manager, _ names.ModuleName) error {
	go e.readLoop()
	return nil
}

// AfterStop implements the ModifierAfterStop interface
func (e *EventHandler) AfterStop(_ *manager.Manager, _ names.ModuleName, _ manager.MapCleanupType) error {
	if e.perfChan != nil {
		close(e.perfChan)
	}
	if e.ringChan != nil {
		close(e.ringChan)
	}
	return nil
}

func (e *EventHandler) String() string {
	return "EventHandler"
}

// Flush flushes the pending data from the underlying perfbuf/ringbuf
func (e *EventHandler) Flush() {
	e.f.Flush()
}

func updateMapSpecEditor(mgrOpts *manager.Options, mapName string, editorFunc func(specEditor *manager.MapSpecEditor)) {
	if mgrOpts.MapSpecEditors == nil {
		mgrOpts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	specEditor := mgrOpts.MapSpecEditors[mapName]
	editorFunc(&specEditor)
	mgrOpts.MapSpecEditors[mapName] = specEditor
}

// ResizeRingBuffer resizes the ring buffer by creating/updating a map spec editor
func ResizeRingBuffer(mgrOpts *manager.Options, mapName string, bufferSize int) {
	updateMapSpecEditor(mgrOpts, mapName, func(specEditor *manager.MapSpecEditor) {
		specEditor.MaxEntries = uint32(bufferSize)
		specEditor.EditorFlag |= manager.EditMaxEntries
	})
}

func (e *EventHandler) perfLoop() {
	for record := range e.perfChan {
		e.perfLoopHandler(record)
	}
}

func (e *EventHandler) initPerfBuffer(mgr *manager.Manager) {
	e.perfChan = make(chan *perf.Record, e.opts.channelSize)
	e.readLoop = e.perfLoop

	// remove any existing perf buffers from manager
	mgr.PerfMaps = slices.DeleteFunc(mgr.PerfMaps, func(perfMap *manager.PerfMap) bool {
		return perfMap.Name == e.mapName
	})
	pm := &manager.PerfMap{
		Map: manager.Map{Name: e.mapName},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: e.opts.perfBufferSize,
			Watermark:          e.opts.perfOptions.watermark,
			WakeupEvents:       e.opts.perfOptions.wakeupEvents,
			RecordHandler:      e.perfRecordHandler,
			LostHandler:        nil, // TODO do we need support for Lost?
			RecordGetter:       perfPool.Get,
			TelemetryEnabled:   e.opts.telemetryEnabled,
		},
	}
	mgr.PerfMaps = append(mgr.PerfMaps, pm)
	ebpfTelemetry.ReportPerfMapTelemetry(pm)
	ebpfTelemetry.ReportPerfMapChannelLenTelemetry(pm, func() int {
		return int(e.chLenTelemetry.Swap(0))
	})
	e.f = pm
}

func (e *EventHandler) perfRecordHandler(record *perf.Record, _ *manager.PerfMap, _ *manager.Manager) {
	e.perfChan <- record
	if e.opts.telemetryEnabled {
		updateMaxTelemetry(e.chLenTelemetry, uint64(len(e.perfChan)))
	}
}

func (e *EventHandler) perfLoopHandler(record *perf.Record) {
	// record is only allowed to live for the duration of the callback. Put it back into the sync.Pool once done.
	defer perfPool.Put(record)
	e.handler(record.RawSample)
}

func (e *EventHandler) initRingBuffer(mgr *manager.Manager) {
	e.ringChan = make(chan *ringbuf.Record, e.opts.channelSize)
	e.readLoop = e.ringLoop

	// remove any existing matching ring buffers from manager
	mgr.RingBuffers = slices.DeleteFunc(mgr.RingBuffers, func(ringBuf *manager.RingBuffer) bool {
		return ringBuf.Name == e.mapName
	})
	rb := &manager.RingBuffer{
		Map: manager.Map{Name: e.mapName},
		RingBufferOptions: manager.RingBufferOptions{
			RecordHandler:    e.ringRecordHandler,
			RecordGetter:     ringbufPool.Get,
			TelemetryEnabled: e.opts.telemetryEnabled,
		},
	}
	mgr.RingBuffers = append(mgr.RingBuffers, rb)
	ebpfTelemetry.ReportRingBufferTelemetry(rb)
	ebpfTelemetry.ReportRingBufferChannelLenTelemetry(rb, func() int {
		return int(e.chLenTelemetry.Swap(0))
	})
	e.f = rb
}

func (e *EventHandler) ringLoop() {
	for record := range e.ringChan {
		e.ringLoopHandler(record)
	}
}

func (e *EventHandler) ringRecordHandler(record *ringbuf.Record, _ *manager.RingBuffer, _ *manager.Manager) {
	e.ringChan <- record
	if e.opts.telemetryEnabled {
		updateMaxTelemetry(e.chLenTelemetry, uint64(len(e.ringChan)))
	}
}

func (e *EventHandler) ringLoopHandler(record *ringbuf.Record) {
	// record is only allowed to live for the duration of the callback. Put it back into the sync.Pool once done.
	defer ringbufPool.Put(record)
	e.handler(record.RawSample)
}

// UpgradePerfBuffer upgrades a perf buffer to a ring buffer by creating a map spec editor
func UpgradePerfBuffer(mgr *manager.Manager, mgrOpts *manager.Options, mapName string) {
	if mgrOpts.MapSpecEditors == nil {
		mgrOpts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	specEditor := mgrOpts.MapSpecEditors[mapName]
	specEditor.Type = ebpf.RingBuf
	specEditor.KeySize = 0
	specEditor.ValueSize = 0
	specEditor.EditorFlag |= manager.EditType | manager.EditKeyValue
	mgrOpts.MapSpecEditors[mapName] = specEditor

	// remove map from perf maps because it has been upgraded
	mgr.PerfMaps = slices.DeleteFunc(mgr.PerfMaps, func(perfMap *manager.PerfMap) bool {
		return perfMap.Name == mapName
	})
}

// implement the CAS algorithm to atomically update a max value
func updateMaxTelemetry(a *atomic.Uint64, val uint64) {
	for {
		oldVal := a.Load()
		if val <= oldVal {
			return
		}
		// if the value at a is not `oldVal`, then `CompareAndSwap` returns
		// false indicating that the value of the atomic has changed between
		// the above check and this invocation.
		// In this case we retry the above test, to see if the value still needs
		// to be updated.
		if a.CompareAndSwap(oldVal, val) {
			return
		}
	}
}
