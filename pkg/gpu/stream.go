// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StreamHandler is responsible for receiving events from a single CUDA stream and generating
// kernel spans and memory allocations from them.
type StreamHandler struct {
	key            *model.StreamKey
	kernelLaunches []enrichedKernelLaunch
	memAllocEvents map[uint64]gpuebpf.CudaMemEvent
	kernelSpans    []*model.KernelSpan
	allocations    []*model.MemoryAllocation
	processEnded   bool // A marker to indicate that the process has ended, and this handler should be flushed
	sysCtx         *systemContext
}

type enrichedKernelLaunch struct {
	gpuebpf.CudaKernelLaunch
	kernel *cuda.CubinKernel
}

func newStreamHandler(key *model.StreamKey, sysCtx *systemContext) *StreamHandler {
	return &StreamHandler{
		memAllocEvents: make(map[uint64]gpuebpf.CudaMemEvent),
		key:            key,
		sysCtx:         sysCtx,
	}
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	enrichedLaunch := &enrichedKernelLaunch{
		CudaKernelLaunch: *event, // Copy events, as the memory can be overwritten in the ring buffer after the function returns
	}

	err := sh.tryAttachKernelData(enrichedLaunch)
	if err != nil {
		log.Warnf("Error attaching kernel data: %v", err)
	}

	sh.kernelLaunches = append(sh.kernelLaunches, *enrichedLaunch)
}

func (sh *StreamHandler) tryAttachKernelData(event *enrichedKernelLaunch) error {
	if sh.sysCtx == nil {
		return nil // No system context, kernel data attaching is disabled
	}

	maps, err := sh.sysCtx.getProcessMemoryMaps(int(sh.key.Pid))
	if err != nil {
		return fmt.Errorf("error reading process memory maps: %w", err)
	}

	entry := maps.FindEntryForAddress(event.Kernel_addr)
	if entry == nil {
		return fmt.Errorf("could not find entry for kernel address 0x%x", event.Kernel_addr)
	}

	offsetInFile := event.Kernel_addr - entry.Start + entry.Offset

	binaryPath := fmt.Sprintf("%s/%d/root/%s", sh.sysCtx.procRoot, sh.key.Pid, entry.Path)
	fileData, err := sh.sysCtx.getFileData(binaryPath)
	if err != nil {
		return fmt.Errorf("error getting file data: %w", err)
	}

	symbol, ok := fileData.symbolTable[offsetInFile]
	if !ok {
		return fmt.Errorf("could not find symbol for address 0x%x", event.Kernel_addr)
	}

	kern := fileData.fatbin.GetKernel(symbol, uint32(sh.sysCtx.deviceSmVersions[0]))
	if kern == nil {
		return fmt.Errorf("could not find kernel for symbol %s", symbol)
	}

	event.kernel = kern

	return nil
}

func (sh *StreamHandler) handleMemEvent(event *gpuebpf.CudaMemEvent) {
	if event.Type == gpuebpf.CudaMemAlloc {
		sh.memAllocEvents[event.Addr] = *event
		return
	}

	// We only support alloc and free events for now, so if it's not alloc it's free.
	alloc, ok := sh.memAllocEvents[event.Addr]
	if !ok {
		log.Warnf("Invalid free event: %v", event)
		return
	}

	data := model.MemoryAllocation{
		StartKtime: alloc.Header.Ktime_ns,
		EndKtime:   event.Header.Ktime_ns,
		Size:       alloc.Size,
		Type:       model.GlobalMemAlloc,
		IsLeaked:   false, // Came from a free event, so it's not a leak
	}

	sh.allocations = append(sh.allocations, &data)
	delete(sh.memAllocEvents, event.Addr)
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

func (sh *StreamHandler) getCurrentKernelSpan(maxTime uint64) *model.KernelSpan {
	span := model.KernelSpan{
		StartKtime: math.MaxUint64,
		EndKtime:   maxTime,
		NumKernels: 0,
	}

	for _, launch := range sh.kernelLaunches {
		// Skip launches that happened after the max time we are interested in
		// For example, do not include launches that happened after the synchronization event
		if launch.Header.Ktime_ns >= maxTime {
			continue
		}

		span.StartKtime = min(launch.Header.Ktime_ns, span.StartKtime)
		span.EndKtime = max(launch.Header.Ktime_ns, span.EndKtime)
		blockSize := launch.Block_size.X * launch.Block_size.Y * launch.Block_size.Z
		blockCount := launch.Grid_size.X * launch.Grid_size.Y * launch.Grid_size.Z
		span.AvgThreadCount += uint64(blockSize) * uint64(blockCount)
		span.AvgSharedMem += uint64(launch.Shared_mem_size)

		if launch.kernel != nil {
			span.AvgConstantMem += uint64(launch.kernel.ConstantMem)
			span.AvgSharedMem += uint64(launch.kernel.SharedMem)
			span.AvgKernelSize += uint64(launch.kernel.KernelSize)
		}

		span.NumKernels++
	}

	if span.NumKernels == 0 {
		return nil
	}

	span.AvgThreadCount /= uint64(span.NumKernels)
	span.AvgConstantMem /= uint64(span.NumKernels)
	span.AvgSharedMem /= uint64(span.NumKernels)
	span.AvgKernelSize /= uint64(span.NumKernels)

	return &span
}

func getAssociatedAllocations(span *model.KernelSpan) []*model.MemoryAllocation {
	if span == nil {
		return nil
	}

	allocations := []*model.MemoryAllocation{
		{StartKtime: span.StartKtime, EndKtime: span.EndKtime, Size: span.AvgConstantMem, Type: model.ConstantMemAlloc},
		{StartKtime: span.StartKtime, EndKtime: span.EndKtime, Size: span.AvgSharedMem, Type: model.SharedMemAlloc},
		{StartKtime: span.StartKtime, EndKtime: span.EndKtime, Size: span.AvgKernelSize, Type: model.KernelMemAlloc},
	}

	return allocations
}

// getPastData returns all the events that have finished (kernel spans with synchronizations/allocations that have been freed)
// If flush is true, the data will be cleared from the handler
func (sh *StreamHandler) getPastData(flush bool) *model.StreamData {
	if len(sh.kernelSpans) == 0 && len(sh.allocations) == 0 {
		return nil
	}

	data := &model.StreamData{
		Spans:       sh.kernelSpans,
		Allocations: sh.allocations,
	}

	if flush {
		sh.kernelSpans = nil
		sh.allocations = nil
	}

	return data
}

// getCurrentData returns the current state of the stream (kernels that are still running, and allocations that haven't been freed)
// as this data needs to be treated differently from past/finished data.
func (sh *StreamHandler) getCurrentData(now uint64) *model.StreamData {
	if len(sh.kernelLaunches) == 0 && len(sh.memAllocEvents) == 0 {
		return nil
	}

	data := &model.StreamData{}
	span := sh.getCurrentKernelSpan(now)
	if span != nil {
		data.Spans = append(data.Spans, span)
		data.Allocations = append(data.Allocations, getAssociatedAllocations(span)...)
	}

	for _, alloc := range sh.memAllocEvents {
		data.Allocations = append(data.Allocations, &model.MemoryAllocation{
			StartKtime: alloc.Header.Ktime_ns,
			EndKtime:   0,
			Size:       alloc.Size,
			IsLeaked:   false,
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
	for _, alloc := range sh.memAllocEvents {
		data := model.MemoryAllocation{
			StartKtime: alloc.Header.Ktime_ns,
			EndKtime:   uint64(nowTs),
			Size:       alloc.Size,
			IsLeaked:   true,
		}
		sh.allocations = append(sh.allocations, &data)
	}

	sh.sysCtx.cleanupDataForProcess(int(sh.key.Pid))

	return nil
}
