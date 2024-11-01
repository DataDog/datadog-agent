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

	"github.com/cihub/seelog"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	batchMapSuffix  = "_batches"
	eventsMapSuffix = "_batch_events"
	sizeOfBatch     = int(unsafe.Sizeof(batch{}))
)

var errInvalidPerfEvent = errors.New("invalid perf event")

// Consumer provides a standardized abstraction for consuming (batched) events from eBPF
type Consumer[V any] struct {
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
	metricGroup        *telemetry.MetricGroup
	eventsCount        *telemetry.Counter
	failedFlushesCount *telemetry.Counter
	kernelDropsCount   *telemetry.Counter
	invalidEventsCount *telemetry.Counter
}

// NewConsumer instantiates a new event Consumer
// `callback` is executed once for every "event" generated on eBPF and must:
// 1) copy the data it wishes to hold since the underlying byte array is reclaimed;
// 2) be thread-safe, as the callback may be executed concurrently from multiple go-routines;
func NewConsumer[V any](proto string, ebpf *manager.Manager, callback func([]V)) (*Consumer[V], error) {
	batchMapName := proto + batchMapSuffix
	batchMap, err := maps.GetMap[batchKey, batch](ebpf, batchMapName)
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
	invalidEventsCount := metricGroup.NewCounter("invalid_events")

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

	return &Consumer[V]{
		proto:       proto,
		callback:    callback,
		syncRequest: make(chan chan struct{}),
		offsets:     offsets,
		handler:     handler,
		batchReader: batchReader,

		// telemetry
		metricGroup:        metricGroup,
		eventsCount:        eventsCount,
		failedFlushesCount: failedFlushesCount,
		kernelDropsCount:   kernelDropsCount,
		invalidEventsCount: invalidEventsCount,
	}, nil
}

// Start consumption of eBPF events
func (c *Consumer[V]) Start() {
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
					c.invalidEventsCount.Add(1)
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

				c.batchReader.ReadAll(func(_ int, b *batch) {
					c.process(b, true)
				})
				if log.ShouldLog(seelog.DebugLvl) {
					log.Debugf("usm events summary: name=%q %s", c.proto, c.metricGroup.Summary())
				}
				close(done)
			}
		}
	}()
}

// Sync userpace with kernelspace by fetching all buffered data on eBPF
// Calling this will block until all eBPF map data has been fetched and processed
func (c *Consumer[V]) Sync() {
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
func (c *Consumer[V]) Stop() {
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

func (c *Consumer[V]) process(b *batch, syncing bool) {
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
	if length < 0 || length > int(b.Cap) {
		c.invalidEventsCount.Add(1)
		return
	}

	c.eventsCount.Add(int64(end - begin))

	// generate a slice of type []V from the batch
	ptr := pointerToElement[V](b, begin)
	events := unsafe.Slice(ptr, length)

	c.callback(events)
}

func batchFromEventData(data []byte) (*batch, error) {
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

	return (*batch)(unsafe.Pointer(&data[0])), nil
}

func pointerToElement[V any](b *batch, elementIdx int) *V {
	offset := elementIdx * int(b.Event_size)
	return (*V)(unsafe.Pointer(uintptr(unsafe.Pointer(&b.Data[0])) + uintptr(offset)))
}
