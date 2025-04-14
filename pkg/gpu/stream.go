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
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	lru "github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noSmVersion is used when the SM version is not available
const noSmVersion uint32 = 0

// logLimitStreams is used to limit the number of times we log messages about streams, as that can be very verbose
var logLimitStreams = log.NewLogLimit(20, 5*time.Minute)

// streamLimits contains configurable limits for a stream
type streamLimits struct {
	maxKernelLaunches int
	maxAllocEvents    int
}

// StreamHandler is responsible for receiving events from a single CUDA stream and generating
// kernel spans and memory allocations from them.
type StreamHandler struct {
	metadata       streamMetadata
	kernelLaunches []enrichedKernelLaunch
	memAllocEvents *lru.LRU[uint64, gpuebpf.CudaMemEvent] // holds the memory allocations for the stream, will evict the oldest allocation if the cache is full
	kernelSpans    []*kernelSpan
	allocations    []*memoryAllocation
	processEnded   bool // A marker to indicate that the process has ended, and this handler should be flushed
	sysCtx         *systemContext
	limits         streamLimits
	telemetry      *streamTelemetry // shared telemetry objects for stream-specific telemetry
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

// streamData contains kernel spans and allocations for a stream
type streamData struct {
	spans       []*kernelSpan
	allocations []*memoryAllocation
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

// memoryAllocation represents a memory allocation event
type memoryAllocation struct {
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

func newStreamHandler(metadata streamMetadata, sysCtx *systemContext, limits streamLimits, telemetry *streamTelemetry) (*StreamHandler, error) {
	sh := &StreamHandler{
		sysCtx:    sysCtx,
		metadata:  metadata,
		limits:    limits,
		telemetry: telemetry,
	}

	var err error
	sh.memAllocEvents, err = lru.NewLRU[uint64, gpuebpf.CudaMemEvent](limits.maxAllocEvents, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create memAllocEvents cache: %w", err)
	}

	return sh, nil
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	enrichedLaunch := &enrichedKernelLaunch{
		CudaKernelLaunch: *event, // Copy events, as the memory can be overwritten in the ring buffer after the function returns
		stream:           sh,
	}

	// Trigger the background kernel data loading, we don't care about the result here
	_, err := enrichedLaunch.getKernelData()
	if err != nil && !errors.Is(err, cuda.ErrKernelNotProcessedYet) && !errors.Is(err, errFatbinParsingDisabled) { // Only log the error if it's not the retryable error
		if logLimitStreams.ShouldLog() {
			log.Warnf("Error attaching kernel data for PID %d: %v", sh.metadata.pid, err)
		}
	}

	sh.kernelLaunches = append(sh.kernelLaunches, *enrichedLaunch)

	// If we've reached the kernel launch limit, trigger a sync. This stops us from just collecting
	// kernel launches and not generating any spans if for some reason we are missing sync events.
	if len(sh.kernelLaunches) >= sh.limits.maxKernelLaunches {
		sh.markSynchronization(event.Header.Ktime_ns + 1) // sync "happens" after the launch, not the same time. If the time is the same, the last kernel launch is not included in the span
		sh.telemetry.forcedSyncOnKernelLaunch.Inc()
	}
}

func (sh *StreamHandler) handleMemEvent(event *gpuebpf.CudaMemEvent) {
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
		if logLimitStreams.ShouldLog() {
			log.Warnf("Invalid free event: %v", event)
		}
		sh.telemetry.invalidFreeEvents.Inc()
		return
	}

	data := memoryAllocation{
		startKtime: alloc.Header.Ktime_ns,
		endKtime:   event.Header.Ktime_ns,
		size:       alloc.Size,
		allocType:  globalMemAlloc,
		isLeaked:   false,
	}

	sh.allocations = append(sh.allocations, &data)
	sh.memAllocEvents.Remove(event.Addr)
}

func (sh *StreamHandler) markSynchronization(ts uint64) {
	span := sh.getCurrentKernelSpan(ts)
	if span == nil {
		return
	}

	sh.kernelSpans = append(sh.kernelSpans, span)
	sh.allocations = append(sh.allocations, getAssociatedAllocations(span)...)

	remainingLaunches := []enrichedKernelLaunch{}
	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= ts {
			remainingLaunches = append(remainingLaunches, launch)
		}
	}
	sh.kernelLaunches = remainingLaunches
}

func (sh *StreamHandler) handleSync(event *gpuebpf.CudaSync) {
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
			if !errors.Is(err, errFatbinParsingDisabled) && logLimitStreams.ShouldLog() {
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

func getAssociatedAllocations(span *kernelSpan) []*memoryAllocation {
	if span == nil {
		return nil
	}

	allocations := make([]*memoryAllocation, 0, len(span.avgMemoryUsage))
	for allocType, size := range span.avgMemoryUsage {
		allocations = append(allocations, &memoryAllocation{
			startKtime: span.startKtime,
			endKtime:   span.endKtime,
			size:       size,
			isLeaked:   false,
			allocType:  allocType,
		})
	}

	return allocations
}

// getPastData returns all the events that have finished (kernel spans with synchronizations/allocations that have been freed)
// If flush is true, the data will be cleared from the handler
func (sh *StreamHandler) getPastData(flush bool) *streamData {
	if len(sh.kernelSpans) == 0 && len(sh.allocations) == 0 {
		return nil
	}

	data := &streamData{
		spans:       sh.kernelSpans,
		allocations: sh.allocations,
	}

	if flush {
		sh.kernelSpans = nil
		sh.allocations = nil
	}

	return data
}

// getCurrentData returns the current state of the stream (kernels that are still running, and allocations that haven't been freed)
// as this data needs to be treated differently from past/finished data.
func (sh *StreamHandler) getCurrentData(now uint64) *streamData {
	if len(sh.kernelLaunches) == 0 && sh.memAllocEvents.Len() == 0 {
		return nil
	}

	data := &streamData{}
	span := sh.getCurrentKernelSpan(now)
	if span != nil {
		data.spans = append(data.spans, span)
		data.allocations = append(data.allocations, getAssociatedAllocations(span)...)
	}

	for alloc := range sh.memAllocEvents.ValuesIter() {
		data.allocations = append(data.allocations, &memoryAllocation{
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

	sh.processEnded = true
	sh.markSynchronization(uint64(nowTs))

	// Close all allocations. Treat them as leaks, as they weren't freed properly
	for alloc := range sh.memAllocEvents.ValuesIter() {
		data := memoryAllocation{
			startKtime: alloc.Header.Ktime_ns,
			endKtime:   uint64(nowTs),
			size:       alloc.Size,
			isLeaked:   true,
			allocType:  globalMemAlloc,
		}
		sh.allocations = append(sh.allocations, &data)
	}

	sh.sysCtx.removeProcess(int(sh.metadata.pid))

	return nil
}
