// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	batchMapSuffix  = "_batches"
	eventsMapSuffix = "_batch_events"
	sizeOfBatch     = int(unsafe.Sizeof(Batch{}))
)

var errInvalidPerfEvent = errors.New("invalid perf event")

// Consumer is an interface for event consumers
type Consumer[V any] interface {
	Start()
	Sync()
	Stop()
}

// DirectConsumer processes individual events directly from eBPF programs.
type DirectConsumer[V any] struct {
	perf.EventHandler
	proto    string
	callback func(*V)

	// lifecycle management
	once   sync.Once
	closed chan struct{}

	// flush coordination
	flushRequests    chan chan struct{}
	flushChannel     chan chan struct{}
	originalCallback func(*V)

	// telemetry
	metricGroup       *telemetry.MetricGroup
	eventsCount       *telemetry.Counter
	invalidEventCount *telemetry.Counter
}

// NewDirectConsumer creates a new DirectConsumer for the specified protocol.
func NewDirectConsumer[V any](proto string, callback func(*V), config *config.Config) (*DirectConsumer[V], error) {
	if callback == nil {
		return nil, errors.New("callback function is required")
	}

	// setup telemetry
	metricGroup := telemetry.NewMetricGroup(
		fmt.Sprintf("usm.%s", proto),
		telemetry.OptStatsd,
	)
	eventsCount := metricGroup.NewCounter("events_captured")
	invalidEventCount := metricGroup.NewCounter("invalid_event_count")

	consumer := &DirectConsumer[V]{
		proto:             proto,
		callback:          callback,
		originalCallback:  callback,
		closed:            make(chan struct{}),
		flushRequests:     make(chan chan struct{}),
		flushChannel:      make(chan chan struct{}, 1),
		metricGroup:       metricGroup,
		eventsCount:       eventsCount,
		invalidEventCount: invalidEventCount,
	}

	// Create handler function that processes individual events and handles flush coordination
	handler := func(data []byte) {
		// Handle sentinel flush record (empty data indicates flush completion)
		if len(data) == 0 {
			select {
			case request := <-consumer.flushChannel:
				close(request) // Signal flush completion
			default:
				// No pending flush request, ignore sentinel
			}
			return
		}

		if len(data) < int(unsafe.Sizeof(*new(V))) {
			consumer.invalidEventCount.Add(1)
			log.Debugf("DirectConsumer %s: received data too small for event type, size: %d, expected: %d",
				proto, len(data), int(unsafe.Sizeof(*new(V))))
			return
		}

		consumer.eventsCount.Add(1)

		// Convert raw bytes to typed event
		event := (*V)(unsafe.Pointer(&data[0]))
		consumer.callback(event)
	}

	// Set up perf mode and channel size similar to initClosedConnEventHandler
	// For DirectConsumer, we want kernel-level batching for performance
	// but individual event processing in userspace (unlike BatchConsumer)
	perfMode := perf.WakeupEvents(config.USMDirectBufferWakeupCount) // Wait for N events before wakeup
	chanSize := config.USMDirectChannelSize * config.USMDirectBufferWakeupCount

	mode := perf.UsePerfBuffers(config.USMDirectPerfBufferSize, chanSize, perfMode)
	// Always try to upgrade to ring buffers for direct events if supported
	if config.RingBufferSupportedUSM() {
		mode = perf.UpgradePerfBuffers(config.USMDirectPerfBufferSize, chanSize, perfMode, config.USMDirectRingBufferSize)
	}

	// Calculate the size of the single event that will be written via bpf_ringbuf_output
	eventSize := int(unsafe.Sizeof(*new(V)))

	mapName := proto + eventsMapSuffix
	eventHandler, err := perf.NewEventHandler(
		mapName,
		handler,
		mode,
		perf.SendTelemetry(config.InternalTelemetryEnabled),
		perf.RingBufferEnabledConstantName("ringbuffers_enabled"),
		perf.RingBufferWakeupSize("ringbuffer_wakeup_size",
			uint64(config.USMDirectBufferWakeupCount*(eventSize+unix.BPF_RINGBUF_HDR_SZ))),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event handler for protocol %s: %w", proto, err)
	}

	consumer.EventHandler = *eventHandler

	return consumer, nil
}

// Start implements the Consumer interface
// Note: The embedded EventHandler must be passed to the eBPF manager during initialization
// The manager will call PreStart() to start the read loop automatically
func (c *DirectConsumer[V]) Start() {
	if c == nil {
		return
	}
	// Start flush coordination goroutine
	go c.flushCoordinator()

	// The eBPF manager will call the EventHandler's PreStart method
	// when the manager starts. This happens automatically through the modifier interface.
	log.Debugf("DirectConsumer: starting for protocol %s", c.proto)
}

// Sync implements Consumer interface
func (c *DirectConsumer[V]) Sync() {
	if c == nil {
		return
	}

	// Check if already closed
	select {
	case <-c.closed:
		return
	default:
	}

	// Create completion channel and send flush request
	wait := make(chan struct{})
	select {
	case <-c.closed:
		return
	case c.flushRequests <- wait:
		// Wait for flush completion
		<-wait
	default:
		// If flush channel is full, skip (already flushing)
	}
}

// Stop implements Consumer interface
func (c *DirectConsumer[V]) Stop() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.closed)
	})
	// The EventHandler's AfterStop method handles cleanup
	// This would typically be called by the ebpf-manager during shutdown
	log.Debugf("DirectConsumer: stopping for protocol %s", c.proto)
}

// flushCoordinator coordinates synchronous flushes based on tcp_close_consumer pattern
func (c *DirectConsumer[V]) flushCoordinator() {
	for {
		select {
		case <-c.closed:
			return
		case request := <-c.flushRequests:
			// Send the completion channel to flushChannel for callback coordination
			c.flushChannel <- request
			// Call the underlying EventHandler.Flush() to force ring buffer flush
			// This will cause the callback to eventually receive a sentinel record
			c.EventHandler.Flush()
		}
	}
}

// BatchConsumer processes batches of events from eBPF maps.
type BatchConsumer[V any] struct {
	mux         sync.Mutex
	proto       string
	syncRequest chan (chan struct{})
	offsets     *offsetManager
	handler     ddebpf.EventHandler
	batchReader *batchReader
	callback    func([]V)

	// termination
	eventLoopWG sync.WaitGroup
	stopped     bool

	// telemetry
	metricGroup                                        *telemetry.MetricGroup
	eventsCount                                        *telemetry.Counter
	failedFlushesCount                                 *telemetry.Counter
	kernelDropsCount                                   *telemetry.Counter
	lengthExceededEventCount, negativeLengthEventCount *telemetry.Counter
	invalidBatchCount                                  *telemetry.Counter
}

// NewBatchConsumer instantiates a new event BatchConsumer
// `callback` is executed once for every "event" generated on eBPF and must:
// 1) copy the data it wishes to hold since the underlying byte array is reclaimed;
// 2) be thread-safe, as the callback may be executed concurrently from multiple go-routines;
func NewBatchConsumer[V any](proto string, ebpf *manager.Manager, callback func([]V)) (*BatchConsumer[V], error) {
	batchMapName := proto + batchMapSuffix
	batchMap, err := maps.GetMap[batchKey, Batch](ebpf, batchMapName)
	if err != nil {
		return nil, fmt.Errorf("unable to find map %s: %s", batchMapName, err)
	}

	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		numCPUs = 96
		log.Errorf("unable to detect number of CPUs. assuming 96 cores: %s", err)
	}

	offsets := newOffsetManager(numCPUs)
	batchReader, err := newBatchReader(offsets, batchMap, numCPUs)
	if err != nil {
		return nil, err
	}

	handler := getHandler(proto)
	if handler == nil {
		return nil, fmt.Errorf("unable to detect perf handler. perhaps you forgot to call events.Configure()?")
	}

	// setup telemetry
	metricGroup := telemetry.NewMetricGroup(
		fmt.Sprintf("usm.%s", proto),
		telemetry.OptStatsd,
	)

	eventsCount := metricGroup.NewCounter("events_captured")
	kernelDropsCount := metricGroup.NewCounter("kernel_dropped_events")
	negativeLengthEventCount := metricGroup.NewCounter("out_of_bounds_event_count", "type:negative_length")
	lengthExceededEventCount := metricGroup.NewCounter("out_of_bounds_event_count", "type:length_exceeded")
	invalidBatchCount := metricGroup.NewCounter("invalid_batch_count")

	// failedFlushesCount tracks the number of failed calls to
	// `bpf_perf_event_output`. This is usually indicative of a slow-consumer
	// problem, because flushing a perf event will fail when there is no space
	// available in the perf ring. Having said that, in the context of this
	// library a failed call to `bpf_perf_event_output` won't necessarily
	// translate into data drop, because this library will retry flushing a
	// given batch *until the call to `bpf_perf_event_output` succeeds*.  This
	// is OK (in terms of no datapoints being dropped) as long as we have enough
	// event "slots" in other batch pages while the retrying happens.
	//
	// The exact number of events dropped can be obtained using the metric
	// `kernel_dropped_events`.
	failedFlushesCount := metricGroup.NewCounter("failed_flushes")

	return &BatchConsumer[V]{
		proto:       proto,
		callback:    callback,
		syncRequest: make(chan chan struct{}),
		offsets:     offsets,
		handler:     handler,
		batchReader: batchReader,

		// telemetry
		metricGroup:              metricGroup,
		eventsCount:              eventsCount,
		failedFlushesCount:       failedFlushesCount,
		kernelDropsCount:         kernelDropsCount,
		negativeLengthEventCount: negativeLengthEventCount,
		lengthExceededEventCount: lengthExceededEventCount,
		invalidBatchCount:        invalidBatchCount,
	}, nil
}

// Start consumption of eBPF events
func (c *BatchConsumer[V]) Start() {
	c.eventLoopWG.Add(1)
	go func() {
		defer c.eventLoopWG.Done()
		dataChannel := c.handler.DataChannel()
		lostChannel := c.handler.LostChannel()
		for {
			select {
			case dataEvent, ok := <-dataChannel:
				if !ok {
					return
				}

				b, err := batchFromEventData(dataEvent.Data)

				if err != nil {
					c.invalidBatchCount.Add(1)
					dataEvent.Done()
					break
				}

				c.failedFlushesCount.Add(int64(b.Failed_flushes))
				c.kernelDropsCount.Add(int64(b.Dropped_events))
				c.process(b, false)
				dataEvent.Done()
			case _, ok := <-lostChannel:
				if !ok {
					return
				}

				// we have our own telemetry to track failed flushes so we don't
				// do anything here other than draining this channel
			case done, ok := <-c.syncRequest:
				if !ok {
					return
				}

				c.batchReader.ReadAll(func(_ int, b *Batch) {
					c.process(b, true)
				})
				if log.ShouldLog(log.DebugLvl) {
					log.Debugf("usm events summary: name=%q %s", c.proto, c.metricGroup.Summary())
				}
				close(done)
			}
		}
	}()
}

// Sync userpace with kernelspace by fetching all buffered data on eBPF
// Calling this will block until all eBPF map data has been fetched and processed
func (c *BatchConsumer[V]) Sync() {
	c.mux.Lock()
	if c.stopped {
		c.mux.Unlock()
		return
	}

	request := make(chan struct{})
	c.syncRequest <- request
	c.mux.Unlock()

	// Wait until all data is fetch from eBPF
	<-request
}

// Stop consuming data from eBPF
func (c *BatchConsumer[V]) Stop() {
	c.mux.Lock()
	defer c.mux.Unlock()

	if c.stopped {
		return
	}

	c.stopped = true
	c.batchReader.Stop()
	c.handler.Stop()
	c.eventLoopWG.Wait()
	close(c.syncRequest)
}

func (c *BatchConsumer[V]) process(b *Batch, syncing bool) {
	cpu := int(b.Cpu)

	// Determine the subset of data we're interested in as we might have read
	// part of this batch before during a Sync() call
	begin, end := c.offsets.Get(cpu, b, syncing)
	length := end - begin

	// This can happen in the context of a low-traffic host
	// (that is, when no events are enqueued in a batch between two consecutive
	// calls to `Sync()`)
	if length == 0 {
		return
	}

	// Sanity check. Ideally none of these conditions should evaluate to
	// true. In case they do we bail out and increment the counter tracking
	// invalid events
	// TODO: investigate why we're sometimes getting invalid offsets
	if length < 0 {
		c.negativeLengthEventCount.Add(1)
		return
	}
	if length > int(b.Cap) {
		c.lengthExceededEventCount.Add(1)
		return
	}

	c.eventsCount.Add(int64(end - begin))

	// generate a slice of type []V from the batch
	ptr := pointerToElement[V](b, begin)
	events := unsafe.Slice(ptr, length)

	c.callback(events)
}

func batchFromEventData(data []byte) (*Batch, error) {
	if len(data) < sizeOfBatch {
		// For some reason the eBPF program sent us a perf event with a size
		// different from what we're expecting.
		//
		// TODO: we're not ensuring that len(data) == sizeOfBatch, because we're
		// consistently getting events that have a few bytes more than
		// `sizeof(batch_event_t)`. I haven't determined yet where these extra
		// bytes are coming from, but I already validated that is not padding
		// coming from the clang/LLVM toolchain for alignment purposes, so it's
		// something happening *after* the call to bpf_perf_event_output.
		return nil, errInvalidPerfEvent
	}

	return (*Batch)(unsafe.Pointer(&data[0])), nil
}

func pointerToElement[V any](b *Batch, elementIdx int) *V {
	offset := elementIdx * int(b.Event_size)
	return (*V)(unsafe.Pointer(uintptr(unsafe.Pointer(&b.Data[0])) + uintptr(offset)))
}

// KernelAdaptiveConsumer wraps either DirectConsumer or BatchConsumer based on kernel version
// and provides both Consumer interface and Modifier interface in a single struct
type KernelAdaptiveConsumer[V any] struct {
	Consumer[V]                   // Embedded interface for Start/Sync/Stop
	modifiers   []ddebpf.Modifier // Modifiers for eBPF manager
}

// Modifiers implements the ModifierProvider interface
func (k *KernelAdaptiveConsumer[V]) Modifiers() []ddebpf.Modifier {
	return k.modifiers
}

// NewKernelAdaptiveConsumer creates a new KernelAdaptiveConsumer with the given consumer and modifiers
func NewKernelAdaptiveConsumer[V any](consumer Consumer[V], modifiers []ddebpf.Modifier) *KernelAdaptiveConsumer[V] {
	return &KernelAdaptiveConsumer[V]{
		Consumer:  consumer,
		modifiers: modifiers,
	}
}
