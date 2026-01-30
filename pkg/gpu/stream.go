// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	lru "github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// noSmVersion is used when the SM version is not available
const noSmVersion uint32 = 0

// StreamHandler is responsible for receiving events from a single CUDA stream and generating
// kernel spans and memory allocations from them.
type StreamHandler struct {
	metadata            streamMetadata
	kernelLaunchesMutex sync.RWMutex
	kernelLaunches      []*enrichedKernelLaunch
	memAllocEvents      *lru.LRU[uint64, gpuebpf.CudaMemEvent] // holds the memory allocations for the stream, will evict the oldest allocation if the cache is full
	pendingKernelSpans  chan *kernelSpan                       // holds already finalized kernel spans that still need to be collected
	pendingMemorySpans  chan *memorySpan                       // holds already finalized memory allocations that still need to be collected
	ended               bool                                   // A marker to indicate that the stream has ended, and this handler should be flushed
	sysCtx              *systemContext
	config              config.StreamConfig
	telemetry           *streamTelemetry // shared telemetry objects for stream-specific telemetry
	lastEventKtimeNs    uint64           // The kernel-time timestamp of the last event processed by this handler
}

// streamMetadata contains metadata about a CUDA stream
type streamMetadata struct {
	// pid is the PID of the process that is running this stream
	pid uint32

	// streamID is the ID of the CUDA stream
	streamID uint64

	// gpuUUID is the UUID of the GPU this stream is running on
	gpuUUID string

	// containerID is the container ID of the process that is running this stream. Might be empty if the container ID is not available
	// or if the process is not running inside a container
	containerID string

	// smVersion is the SM version of the GPU this stream is running on, for kernel data attaching
	smVersion uint32
}

// streamSpans contains kernel spans and allocations for a stream
type streamSpans struct {
	kernels     []*kernelSpan
	allocations []*memorySpan
}

// releaseSpans releases the spans back to the pool
func (s *streamSpans) releaseSpans() {
	for _, kernel := range s.kernels {
		memPools.kernelSpanPool.Put(kernel)
	}
	for _, allocation := range s.allocations {
		memPools.memorySpanPool.Put(allocation)
	}
}

type memAllocType int

const (
	// kernelMemAlloc represents allocations due to kernel binary size
	kernelMemAlloc memAllocType = iota

	// globalMemAlloc represents allocations due to global memory
	globalMemAlloc

	// sharedMemAlloc represents allocations in shared memory space
	sharedMemAlloc

	// constantMemAlloc represents allocations in constant memory space
	constantMemAlloc

	// memAllocTypeCount is the maximum number of memory allocation types
	memAllocTypeCount
)

// memorySpan represents a memory allocation event
type memorySpan struct {
	// Start is the kernel-time timestamp of the allocation event
	startKtime uint64

	// End is the kernel-time timestamp of the deallocation event. If 0, this means the memory was not deallocated yet
	endKtime uint64

	// size is the size of the allocation in bytes
	size uint64

	// isLeaked is true if the allocation was not deallocated
	isLeaked bool

	// allocType is the type of the allocation
	allocType memAllocType
}

// kernelSpan represents a span of time during which one or more kernels were
// running on a GPU until a synchronization event happened
type kernelSpan struct {
	// startKtime is the kernel-time timestamp of the start of the span, the moment the first kernel was launched
	startKtime uint64

	// endKtime is the kernel-time timestamp of the end of the span, the moment the synchronization event happened
	endKtime uint64

	// avgThreadCount is the average number of threads running on the GPU during the span
	avgThreadCount uint64

	// numKernels is the number of kernels that were launched during the span
	numKernels uint64

	// avgMemoryUsage is the average memory usage during the span, per allocation type
	avgMemoryUsage map[memAllocType]uint64
}

// enrichedKernelLaunch is a structure that wraps a kernel launch event with the code to get
// the kernel data from the kernel cache, in the background
type enrichedKernelLaunch struct {
	gpuebpf.CudaKernelLaunch
	kernel *cuda.CubinKernel
	err    error
	stream *StreamHandler
}

var errFatbinParsingDisabled = errors.New("fatbin parsing is disabled")

// memoryPools is a struct that contains the pools for commonly allocated
// objects, to avoid constant reallocation in high throughput pipelines.
type memoryPools struct {
	enrichedKernelLaunchPool ddsync.Pool[enrichedKernelLaunch]
	kernelSpanPool           ddsync.Pool[kernelSpan]
	memorySpanPool           ddsync.Pool[memorySpan]
	initOnce                 sync.Once
}

var memPools memoryPools

func (m *memoryPools) ensureInit(tm telemetry.Component) {
	m.initOnce.Do(func() {
		m.enrichedKernelLaunchPool = ddsync.NewDefaultTypedPoolWithTelemetry[enrichedKernelLaunch](tm, "gpu", "enrichedKernelLaunch")
		m.kernelSpanPool = ddsync.NewDefaultTypedPoolWithTelemetry[kernelSpan](tm, "gpu", "kernelSpan")
		m.memorySpanPool = ddsync.NewDefaultTypedPoolWithTelemetry[memorySpan](tm, "gpu", "memorySpan")
	})
}

// getKernelData attempts to get the kernel data from the kernel cache.
// If the kernel is not processed yet, it will return errKernelNotProcessedYet, retry later in that case.
// If fatbin parsing is disabled, it will return errFatbinParsingDisabled.
func (e *enrichedKernelLaunch) getKernelData() (*cuda.CubinKernel, error) {
	if e.stream.sysCtx.cudaKernelCache == nil || e.stream.metadata.smVersion == noSmVersion {
		// Fatbin parsing is disabled, so we don't need to get the kernel data.
		// Same is true if we haven't been able to detect the SM version for this stream
		return nil, errFatbinParsingDisabled
	}

	// Use direct comparison instead of errors.Is() for performance in the hot path.
	// Both cuda.ErrKernelNotProcessedYet and errFatbinParsingDisabled are sentinel errors
	// that are never wrapped, so direct comparison is safe and faster.
	// See TestGetKernelDataReturnsUnwrappedErrors for tests ensuring errors are not wrapped.
	if e.kernel != nil || (e.err != nil && e.err != cuda.ErrKernelNotProcessedYet) {
		return e.kernel, e.err
	}

	e.kernel, e.err = e.stream.sysCtx.cudaKernelCache.Get(int(e.stream.metadata.pid), e.Kernel_addr, e.stream.metadata.smVersion)
	return e.kernel, e.err
}

func newStreamHandler(metadata streamMetadata, sysCtx *systemContext, config config.StreamConfig, telemetry *streamTelemetry) (*StreamHandler, error) {
	sh := &StreamHandler{
		sysCtx:    sysCtx,
		metadata:  metadata,
		config:    config,
		telemetry: telemetry,
	}

	var err error
	sh.memAllocEvents, err = lru.NewLRU[uint64, gpuebpf.CudaMemEvent](config.MaxMemAllocEvents, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create memAllocEvents cache: %w", err)
	}

	sh.pendingKernelSpans = make(chan *kernelSpan, config.MaxPendingKernelSpans)
	sh.pendingMemorySpans = make(chan *memorySpan, config.MaxPendingMemorySpans)

	return sh, nil
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	sh.lastEventKtimeNs = event.Header.Ktime_ns

	enrichedLaunch := memPools.enrichedKernelLaunchPool.Get()
	enrichedLaunch.CudaKernelLaunch = *event // Copy events, as the memory can be overwritten in the ring buffer after the function returns
	enrichedLaunch.stream = sh
	enrichedLaunch.kernel = nil
	enrichedLaunch.err = nil

	// Trigger the background kernel data loading, we don't care about the result here
	_, err := enrichedLaunch.getKernelData()
	// Use direct comparison instead of errors.Is() for performance in the hot path.
	// Both cuda.ErrKernelNotProcessedYet and errFatbinParsingDisabled are sentinel errors
	// that are never wrapped, so direct comparison is safe and faster.
	// See TestGetKernelDataReturnsUnwrappedErrors for tests ensuring errors are not wrapped.
	if err != nil && err != cuda.ErrKernelNotProcessedYet && err != errFatbinParsingDisabled && logLimitProbe.ShouldLog() { // Only log the error if it's not the retryable error
		log.Warnf("Error attaching kernel data for PID %d: %v", sh.metadata.pid, err)
	}

	sh.kernelLaunchesMutex.Lock()
	sh.kernelLaunches = append(sh.kernelLaunches, enrichedLaunch)
	sh.kernelLaunchesMutex.Unlock()

	// If we've reached the kernel launch limit, trigger a sync. This stops us from just collecting
	// kernel launches and not generating any spans if for some reason we are missing sync events.
	if len(sh.kernelLaunches) >= sh.config.MaxKernelLaunches {
		sh.markSynchronization(event.Header.Ktime_ns + 1) // sync "happens" after the launch, not the same time. If the time is the same, the last kernel launch is not included in the span
		sh.telemetry.forcedSyncOnKernelLaunch.Inc()
	}
}

// trySendToChannel attempts to send an item to a channel in a non-blocking way, if the channel is full
// it will increment the rejectedSpans telemetry counter
func trySendSpan[T any](sh *StreamHandler, ch chan *T, item *T, pool ddsync.Pool[T]) {
	select {
	case ch <- item:
		return
	default:
		pool.Put(item)
		sh.telemetry.rejectedSpans.Inc()
		return
	}
}

func (sh *StreamHandler) handleMemEvent(event *gpuebpf.CudaMemEvent) {
	sh.lastEventKtimeNs = event.Header.Ktime_ns

	if event.Type == gpuebpf.CudaMemAlloc {
		evicted := sh.memAllocEvents.Add(event.Addr, *event)
		if evicted {
			sh.telemetry.allocEvicted.Inc()
		}
		return
	}

	// We only support alloc and free events for now, so if it's not alloc it's free.
	alloc, ok := sh.memAllocEvents.Get(event.Addr)
	if !ok {
		if logLimitProbe.ShouldLog() {
			log.Warnf("Invalid free event: %v", event)
		}
		sh.telemetry.invalidFreeEvents.Inc()
		return
	}

	data := memPools.memorySpanPool.Get()
	data.startKtime = alloc.Header.Ktime_ns
	data.endKtime = event.Header.Ktime_ns
	data.size = alloc.Size
	data.allocType = globalMemAlloc
	data.isLeaked = false

	trySendSpan(sh, sh.pendingMemorySpans, data, memPools.memorySpanPool)
	sh.memAllocEvents.Remove(event.Addr)
}

func (sh *StreamHandler) markSynchronization(ts uint64) {
	span := sh.getCurrentKernelSpan(ts)
	if span == nil {
		return
	}

	trySendSpan(sh, sh.pendingKernelSpans, span, memPools.kernelSpanPool)
	for _, alloc := range getAssociatedAllocations(span) {
		trySendSpan(sh, sh.pendingMemorySpans, alloc, memPools.memorySpanPool)
	}

	sh.kernelLaunchesMutex.Lock()
	defer sh.kernelLaunchesMutex.Unlock()
	remainingLaunches := []*enrichedKernelLaunch{}
	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= ts {
			remainingLaunches = append(remainingLaunches, launch)
		} else {
			memPools.enrichedKernelLaunchPool.Put(launch)
		}
	}
	sh.kernelLaunches = remainingLaunches
}

func (sh *StreamHandler) handleSync(event *gpuebpf.CudaSync) {
	sh.lastEventKtimeNs = event.Header.Ktime_ns

	// TODO: Worry about concurrent calls to this?
	sh.markSynchronization(event.Header.Ktime_ns)
}

func (sh *StreamHandler) getCurrentKernelSpan(maxTime uint64) *kernelSpan {
	span := memPools.kernelSpanPool.Get()
	span.startKtime = math.MaxUint64
	span.endKtime = maxTime
	span.numKernels = 0
	span.avgThreadCount = 0

	// Reset the memory usage map
	for allocType := range span.avgMemoryUsage {
		span.avgMemoryUsage[allocType] = 0
	}

	if span.avgMemoryUsage == nil {
		span.avgMemoryUsage = make(map[memAllocType]uint64)
	}

	sh.kernelLaunchesMutex.RLock()
	defer sh.kernelLaunchesMutex.RUnlock()

	for _, launch := range sh.kernelLaunches {
		// Skip launches that happened after the max time we are interested in
		// For example, do not include launches that happened after the synchronization event
		if launch.Header.Ktime_ns >= maxTime {
			continue
		}

		span.startKtime = min(launch.Header.Ktime_ns, span.startKtime)
		span.endKtime = max(launch.Header.Ktime_ns, span.endKtime)
		blockSize := launch.Block_size.X * launch.Block_size.Y * launch.Block_size.Z
		blockCount := launch.Grid_size.X * launch.Grid_size.Y * launch.Grid_size.Z
		span.avgThreadCount += uint64(blockSize) * uint64(blockCount)
		span.avgMemoryUsage[sharedMemAlloc] += uint64(launch.Shared_mem_size)

		kernel, err := launch.getKernelData()
		if err != nil && err != errFatbinParsingDisabled && err != cuda.ErrKernelNotProcessedYet && logLimitProbe.ShouldLog() {
			// Use direct comparison instead of errors.Is() for performance in the hot path.
			// errFatbinParsingDisabled is a sentinel error that is never wrapped,
			// so direct comparison is safe and faster.
			// See TestGetKernelDataReturnsUnwrappedErrors for tests ensuring errors are not wrapped.

			log.Warnf("Error getting kernel data for PID %d: %v", sh.metadata.pid, err)
		} else if kernel != nil {
			span.avgMemoryUsage[constantMemAlloc] += uint64(kernel.ConstantMem)
			span.avgMemoryUsage[sharedMemAlloc] += uint64(kernel.SharedMem)
			span.avgMemoryUsage[kernelMemAlloc] += uint64(kernel.KernelSize)
		}

		span.numKernels++
	}

	if span.numKernels == 0 {
		memPools.kernelSpanPool.Put(span)
		return nil
	}

	span.avgThreadCount /= uint64(span.numKernels)
	for allocType := range span.avgMemoryUsage {
		span.avgMemoryUsage[allocType] /= uint64(span.numKernels)
	}

	return span
}

func getAssociatedAllocations(span *kernelSpan) []*memorySpan {
	if span == nil {
		return nil
	}

	allocations := make([]*memorySpan, 0, len(span.avgMemoryUsage))
	for allocType, size := range span.avgMemoryUsage {
		if size == 0 {
			continue
		}

		alloc := memPools.memorySpanPool.Get()
		alloc.startKtime = span.startKtime
		alloc.endKtime = span.endKtime
		alloc.size = size
		alloc.isLeaked = false
		alloc.allocType = allocType
		allocations = append(allocations, alloc)
	}

	return allocations
}

func consumeChannel[T any](ch chan T, count int) []T {
	items := make([]T, 0, count)

	for len(items) < count {
		select {
		case item := <-ch:
			items = append(items, item)
		default:
			// We shouldn't actually hit this, as we should stop consuming the
			// channel when the count is reached and nothing else is consuming
			// from the channel, but break just in case to avoid a deadlock
			return items
		}
	}

	return items
}

// getPastData returns all the events that have finished (kernel spans with synchronizations/allocations that have been freed)
// The data will always be cleared from the handler as it consumes from channels
func (sh *StreamHandler) getPastData() *streamSpans {
	kernelCount := len(sh.pendingKernelSpans)
	allocationCount := len(sh.pendingMemorySpans)

	if kernelCount == 0 && allocationCount == 0 {
		return nil
	}

	data := &streamSpans{
		kernels:     consumeChannel(sh.pendingKernelSpans, kernelCount),
		allocations: consumeChannel(sh.pendingMemorySpans, allocationCount),
	}

	return data
}

// getCurrentData returns the current state of the stream (kernels that are still running, and allocations that haven't been freed)
// as this data needs to be treated differently from past/finished data.
func (sh *StreamHandler) getCurrentData(now uint64) *streamSpans {
	if len(sh.kernelLaunches) == 0 && sh.memAllocEvents.Len() == 0 {
		return nil
	}

	data := &streamSpans{}
	span := sh.getCurrentKernelSpan(now)
	if span != nil {
		data.kernels = append(data.kernels, span)
		data.allocations = append(data.allocations, getAssociatedAllocations(span)...)
	}

	for alloc := range sh.memAllocEvents.ValuesIter() {
		if alloc.Header.Ktime_ns >= now {
			continue
		}

		span := memPools.memorySpanPool.Get()
		span.startKtime = alloc.Header.Ktime_ns
		span.endKtime = 0
		span.size = alloc.Size
		span.allocType = globalMemAlloc
		span.isLeaked = false
		data.allocations = append(data.allocations, span)
	}

	return data
}

// markEnd is called when this stream is closed (process exited or stream destroyed).
// A synchronization event will be triggered and all pending events (allocations) will be resolved.
func (sh *StreamHandler) markEnd() error {
	nowTs, err := ddebpf.NowNanoseconds()
	if err != nil {
		return err
	}

	sh.ended = true
	sh.markSynchronization(uint64(nowTs))

	// Close all allocations. Treat them as leaks, as they weren't freed properly
	for alloc := range sh.memAllocEvents.ValuesIter() {
		data := memPools.memorySpanPool.Get()
		data.startKtime = alloc.Header.Ktime_ns
		data.endKtime = uint64(nowTs)
		data.size = alloc.Size
		data.allocType = globalMemAlloc
		data.isLeaked = true
		trySendSpan(sh, sh.pendingMemorySpans, data, memPools.memorySpanPool)
	}

	sh.sysCtx.removeProcess(int(sh.metadata.pid))

	return nil
}

// releasePoolResources releases all pool-allocated objects held by this handler
// without emitting any spans or data. This is used during cleanup of inactive
// streams where we don't want to generate metrics from stale data.
func (sh *StreamHandler) releasePoolResources() {
	sh.kernelLaunchesMutex.Lock()
	defer sh.kernelLaunchesMutex.Unlock()

	for _, launch := range sh.kernelLaunches {
		memPools.enrichedKernelLaunchPool.Put(launch)
	}
	sh.kernelLaunches = nil

	// Drain and release pending spans from channels.
	// Limit iterations to channel capacities to avoid blocking if concurrent writes happen.
	maxIterations := cap(sh.pendingKernelSpans) + cap(sh.pendingMemorySpans)
	for i := 0; i < maxIterations; i++ {
		select {
		case span := <-sh.pendingKernelSpans:
			memPools.kernelSpanPool.Put(span)
		case span := <-sh.pendingMemorySpans:
			memPools.memorySpanPool.Put(span)
		default:
			return
		}
	}
}

func (sh *StreamHandler) isInactive(now int64, maxInactivity time.Duration) bool {
	// If the stream has no events, it's considered active, we don't want to
	// delete a stream that has just been created
	return sh.lastEventKtimeNs > 0 && now-int64(sh.lastEventKtimeNs) > maxInactivity.Nanoseconds()
}

// String returns a human-readable representation of the StreamHandler. Used for better debugging in tests
func (sh *StreamHandler) String() string {
	sh.kernelLaunchesMutex.RLock()
	kernelLaunchCount := len(sh.kernelLaunches)
	sh.kernelLaunchesMutex.RUnlock()

	return fmt.Sprintf("StreamHandler{pid=%d, streamID=%d, gpu=%s, container=%s, ended=%t, kernelLaunches=%d, pendingKernelSpans=%d, pendingMemorySpans=%d, memAllocEvents=%d}",
		sh.metadata.pid,
		sh.metadata.streamID,
		sh.metadata.gpuUUID,
		sh.metadata.containerID,
		sh.ended,
		kernelLaunchCount,
		len(sh.pendingKernelSpans),
		len(sh.pendingMemorySpans),
		sh.memAllocEvents.Len(),
	)
}
