// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StreamHandler is responsible for receiving events from a single CUDA stream and generating
// stats from them.
type StreamHandler struct {
	kernelLaunches []gpuebpf.CudaKernelLaunch
	memAllocEvents map[uint64]gpuebpf.CudaMemEvent
	kernelSpans    []*model.KernelSpan
	allocations    []*model.MemoryAllocation
	processEnded   bool // A marker to indicate that the process has ended, and this handler should be flushed
}

func newStreamHandler() *StreamHandler {
	return &StreamHandler{
		memAllocEvents: make(map[uint64]gpuebpf.CudaMemEvent),
	}
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	// Copy events, as the memory can be overwritten in the ring buffer after the function returns
	sh.kernelLaunches = append(sh.kernelLaunches, *event)
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

	remainingLaunches := []gpuebpf.CudaKernelLaunch{}
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
		span.NumKernels++
	}

	if span.NumKernels == 0 {
		return nil
	}

	span.AvgThreadCount /= uint64(span.NumKernels)

	return &span
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

func (sh *StreamHandler) getCurrentData(now uint64) *model.StreamData {
	if len(sh.kernelLaunches) == 0 && len(sh.memAllocEvents) == 0 {
		return nil
	}

	data := &model.StreamData{}
	span := sh.getCurrentKernelSpan(now)
	if span != nil {
		data.Spans = append(data.Spans, span)
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

	return nil
}
