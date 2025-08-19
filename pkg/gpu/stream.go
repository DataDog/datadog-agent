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
	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	lru "github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noSmVersion is used when the SM version is not available
const noSmVersion uint32 = 0

// StreamHandler is responsible for receiving events from a single CUDA stream and generating
// kernel spans and memory allocations from them.
type StreamHandler struct {
	metadata           streamMetadata
	kernelLaunches     []enrichedKernelLaunch
	memAllocEvents     *lru.LRU[uint64, gpuebpf.CudaMemEvent] // holds the memory allocations for the stream, will evict the oldest allocation if the cache is full
	pendingKernelSpans chan *kernelSpan                       // holds already finalized kernel spans that still need to be collected
	pendingMemorySpans chan *memorySpan                       // holds already finalized memory allocations that still need to be collected
	ended              bool                                   // A marker to indicate that the stream has ended, and this handler should be flushed
	sysCtx             *systemContext
	config             config.StreamConfig
	telemetry          *streamTelemetry // shared telemetry objects for stream-specific telemetry
	lastEventKtimeNs   uint64           // The kernel-time timestamp of the last event processed by this handler
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

// getKernelData attempts to get the kernel data from the kernel cache.
// If the kernel is not processed yet, it will return errKernelNotProcessedYet, retry later in that case.
// If fatbin parsing is disabled, it will return errFatbinParsingDisabled.
func (e *enrichedKernelLaunch) getKernelData() (*cuda.CubinKernel, error) {
	if e.stream.sysCtx.cudaKernelCache == nil || e.stream.metadata.smVersion == noSmVersion {
		// Fatbin parsing is disabled, so we don't need to get the kernel data.
		// Same is true if we haven't been able to detect the SM version for this stream
		return nil, errFatbinParsingDisabled
	}

	if e.kernel != nil || (e.err != nil && !errors.Is(e.err, cuda.ErrKernelNotProcessedYet)) {
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

	enrichedLaunch := &enrichedKernelLaunch{
		CudaKernelLaunch: *event, // Copy events, as the memory can be overwritten in the ring buffer after the function returns
		stream:           sh,
	}

	// Trigger the background kernel data loading, we don't care about the result here
	_, err := enrichedLaunch.getKernelData()
	if err != nil && !errors.Is(err, cuda.ErrKernelNotProcessedYet) && !errors.Is(err, errFatbinParsingDisabled) { // Only log the error if it's not the retryable error
		if logLimitProbe.ShouldLog() {
			log.Warnf("Error attaching kernel data for PID %d: %v", sh.metadata.pid, err)
		}
	}

	sh.kernelLaunches = append(sh.kernelLaunches, *enrichedLaunch)

	// If we've reached the kernel launch limit, trigger a sync. This stops us from just collecting
	// kernel launches and not generating any spans if for some reason we are missing sync events.
	if len(sh.kernelLaunches) >= sh.config.MaxKernelLaunches {
		sh.markSynchronization(event.Header.Ktime_ns + 1) // sync "happens" after the launch, not the same time. If the time is the same, the last kernel launch is not included in the span
		sh.telemetry.forcedSyncOnKernelLaunch.Inc()
	}
}

// trySendToChannel attempts to send an item to a channel in a non-blocking way, if the channel is full
// it will increment the rejectedSpans telemetry counter
func trySendSpan[T any](sh *StreamHandler, ch chan T, item T) {
	select {
	case ch <- item:
		return
	default:
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

	data := memorySpan{
		startKtime: alloc.Header.Ktime_ns,
		endKtime:   event.Header.Ktime_ns,
		size:       alloc.Size,
		allocType:  globalMemAlloc,
		isLeaked:   false,
	}

	trySendSpan(sh, sh.pendingMemorySpans, &data)
	sh.memAllocEvents.Remove(event.Addr)
}

func (sh *StreamHandler) markSynchronization(ts uint64) {
	span := sh.getCurrentKernelSpan(ts)
	if span == nil {
		return
	}

	trySendSpan(sh, sh.pendingKernelSpans, span)
	for _, alloc := range getAssociatedAllocations(span) {
		trySendSpan(sh, sh.pendingMemorySpans, alloc)
	}

	remainingLaunches := []enrichedKernelLaunch{}
	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= ts {
			remainingLaunches = append(remainingLaunches, launch)
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
	span := kernelSpan{
		startKtime:     math.MaxUint64,
		endKtime:       maxTime,
		numKernels:     0,
		avgMemoryUsage: make(map[memAllocType]uint64),
	}

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
		if err != nil {
			if !errors.Is(err, errFatbinParsingDisabled) && logLimitProbe.ShouldLog() {
				log.Warnf("Error getting kernel data for PID %d: %v", sh.metadata.pid, err)
			}
		} else if kernel != nil {
			span.avgMemoryUsage[constantMemAlloc] += uint64(kernel.ConstantMem)
			span.avgMemoryUsage[sharedMemAlloc] += uint64(kernel.SharedMem)
			span.avgMemoryUsage[kernelMemAlloc] += uint64(kernel.KernelSize)
		}

		span.numKernels++
	}

	if span.numKernels == 0 {
		return nil
	}

	span.avgThreadCount /= uint64(span.numKernels)
	for allocType := range span.avgMemoryUsage {
		span.avgMemoryUsage[allocType] /= uint64(span.numKernels)
	}

	return &span
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

		allocations = append(allocations, &memorySpan{
			startKtime: span.startKtime,
			endKtime:   span.endKtime,
			size:       size,
			isLeaked:   false,
			allocType:  allocType,
		})
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
		data.allocations = append(data.allocations, &memorySpan{
			startKtime: alloc.Header.Ktime_ns,
			endKtime:   0,
			size:       alloc.Size,
			isLeaked:   false,
			allocType:  globalMemAlloc,
		})
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
		data := memorySpan{
			startKtime: alloc.Header.Ktime_ns,
			endKtime:   uint64(nowTs),
			size:       alloc.Size,
			isLeaked:   true,
			allocType:  globalMemAlloc,
		}
		trySendSpan(sh, sh.pendingMemorySpans, &data)
	}

	sh.sysCtx.removeProcess(int(sh.metadata.pid))

	return nil
}

func (sh *StreamHandler) isInactive(now int64, maxInactivity time.Duration) bool {
	// If the stream has no events, it's considered active, we don't want to
	// delete a stream that has just been created
	return sh.lastEventKtimeNs > 0 && now-int64(sh.lastEventKtimeNs) > maxInactivity.Nanoseconds()
}
