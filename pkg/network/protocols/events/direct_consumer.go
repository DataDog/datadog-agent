// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DirectConsumer processes events directly from perf/ring buffers using the in-map batching mechanism
// provided by perf.EventHandler. This is the preferred approach for modern kernels, offering:
// - Watermark-based batching that eliminates custom per-CPU batch pages and race conditions
// - Simplified code by removing batch maps, flush kprobes, and offsetManager complexity
// - Built-in telemetry for lost events, channel lengths, and ring-buffer statistics
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

	// Get CPU count for scaling buffer sizes
	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		numCPUs = 96
		log.Errorf("DirectConsumer %s: unable to detect number of CPUs, assuming 96: %s", proto, err)
	}

	// Set up perf mode and channel size similar to initClosedConnEventHandler
	// For DirectConsumer, we want kernel-level batching for performance
	// but individual event processing in userspace (unlike BatchConsumer)
	perfMode := perf.WakeupEvents(config.USMDirectConsumerBufferWakeupCount) // Wait for N events before wakeup
	chanSize := config.USMDirectConsumerChannelSize * config.USMDirectConsumerBufferWakeupCount

	// Calculate total buffer sizes from per-CPU values
	totalPerfBufferSize := config.USMDirectConsumerPerfBufferSizePerCPU * numCPUs
	totalRingBufferSize := config.USMDirectConsumerRingBufferSizePerCPU * numCPUs

	mode := perf.UsePerfBuffers(totalPerfBufferSize, chanSize, perfMode)
	// Always try to upgrade to ring buffers for direct events if supported
	if config.RingBufferSupportedUSM() {
		mode = perf.UpgradePerfBuffers(totalPerfBufferSize, chanSize, perfMode, totalRingBufferSize)
	}

	// Calculate the size of the single event that will be written via bpf_ringbuf_output
	eventSize := int(unsafe.Sizeof(*new(V)))

	mapName := eventMapName(proto)
	eventHandler, err := perf.NewEventHandler(
		mapName,
		handler,
		mode,
		perf.SendTelemetry(config.InternalTelemetryEnabled),
		perf.RingBufferEnabledConstantName("ringbuffers_enabled"),
		perf.RingBufferWakeupSize("ringbuffer_wakeup_size",
			uint64(config.USMDirectConsumerBufferWakeupCount*(eventSize+unix.BPF_RINGBUF_HDR_SZ))),
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

// SupportsDirectConsumer returns true if the kernel version supports direct consumer (>= 5.8.0)
func SupportsDirectConsumer() bool {
	kernelVersion, err := kernel.HostVersion()
	if err != nil {
		return false
	}
	return kernelVersion >= kernel.VersionCode(5, 8, 0)
}
