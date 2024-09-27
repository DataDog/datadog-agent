// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	kernelLaunches []*gpuebpf.CudaKernelLaunch
	memAllocEvents map[uint64]*gpuebpf.CudaMemEvent
	kernelSpans    []*model.KernelSpan
	allocations    []*model.MemoryAllocation
	processEnded   bool
}

func newStreamHandler() *StreamHandler {
	return &StreamHandler{
		memAllocEvents: make(map[uint64]*gpuebpf.CudaMemEvent),
	}
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	log.Debugf("Handling kernel event: %+v", event)
	sh.kernelLaunches = append(sh.kernelLaunches, event)
}

func (sh *StreamHandler) handleMemEvent(event *gpuebpf.CudaMemEvent) {
	if event.Type == gpuebpf.CudaMemAlloc {
		sh.memAllocEvents[event.Addr] = event
		return
	}

	alloc, ok := sh.memAllocEvents[event.Addr]
	if !ok {
		log.Warnf("Invalid free event: %v", event)
		return
	}

	data := model.MemoryAllocation{
		Start:    alloc.Header.Ktime_ns,
		End:      event.Header.Ktime_ns,
		Size:     alloc.Size,
		IsLeaked: false, // Came from a free event, so it's not a leak
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

	remainingLaunches := []*gpuebpf.CudaKernelLaunch{}
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
		Start:      math.MaxUint64,
		End:        maxTime,
		NumKernels: 0,
	}

	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= maxTime {
			continue
		}

		span.Start = min(launch.Header.Ktime_ns, span.Start)
		span.End = max(launch.Header.Ktime_ns, span.End)
		blockSize := launch.Block_size.X * launch.Block_size.Y * launch.Block_size.Z
		blockCount := launch.Grid_size.X * launch.Grid_size.Y * launch.Grid_size.Z
		span.AvgThreadCount += uint64(blockSize) * uint64(blockCount)
		span.NumKernels++
	}

	if span.NumKernels == 0 {
		return nil
	}

	span.AvgThreadCount /= uint64(span.NumKernels)
	log.Debugf("Current kernel span: %+v", span)

	return &span
}

func (sh *StreamHandler) getPastData(flush bool) *model.StreamPastData {
	if len(sh.kernelSpans) == 0 && len(sh.allocations) == 0 {
		return nil
	}

	data := &model.StreamPastData{
		Spans:       sh.kernelSpans,
		Allocations: sh.allocations,
	}

	if flush {
		sh.kernelSpans = nil
		sh.allocations = nil
	}

	return data
}

func (sh *StreamHandler) getCurrentData(now uint64) *model.StreamCurrentData {
	if len(sh.kernelLaunches) == 0 && len(sh.memAllocEvents) == 0 {
		return nil
	}

	data := &model.StreamCurrentData{
		Span:               sh.getCurrentKernelSpan(now),
		CurrentMemoryUsage: 0,
	}

	for _, alloc := range sh.memAllocEvents {
		data.CurrentAllocations = append(data.CurrentAllocations, &model.MemoryAllocation{
			Start:    alloc.Header.Ktime_ns,
			End:      0,
			Size:     alloc.Size,
			IsLeaked: false,
		})
		data.CurrentMemoryUsage += alloc.Size
	}

	return data
}

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
			Start:    alloc.Header.Ktime_ns,
			End:      uint64(nowTs),
			Size:     alloc.Size,
			IsLeaked: true,
		}
		sh.allocations = append(sh.allocations, &data)
	}

	return nil
}
