// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"math"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type StreamKey struct {
	Pid    uint32 `json:"pid"`
	Tid    uint32 `json:"tid"`
	Stream uint64 `json:"stream"`
}

type StreamHandler struct {
	kernelLaunches []*gpuebpf.CudaKernelLaunch
	memAllocEvents map[uint64]*gpuebpf.CudaMemEvent
	kernelSpans    []*KernelSpan
	allocations    []*MemoryAllocation
	processEnded   bool
}

type KernelSpan struct {
	Start          uint64 `json:"start"`
	End            uint64 `json:"end"`
	AvgThreadCount uint64 `json:"avg_thread_count"`
	NumKernels     uint64 `json:"num_kernels"`
}

type MemoryAllocation struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Size  uint64 `json:"size"`
}

func newStreamHandler() *StreamHandler {
	return &StreamHandler{
		memAllocEvents: make(map[uint64]*gpuebpf.CudaMemEvent),
	}
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
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

	data := MemoryAllocation{
		Start: alloc.Header.Ktime_ns,
		End:   event.Header.Ktime_ns,
		Size:  alloc.Size,
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

func (sh *StreamHandler) getCurrentKernelSpan(maxTime uint64) *KernelSpan {
	span := KernelSpan{
		Start:      math.MaxUint64,
		End:        0,
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

	return &span
}

func (sh *StreamHandler) getCurrentMemoryUsage() uint64 {
	var total uint64
	for _, alloc := range sh.memAllocEvents {
		total += alloc.Size
	}
	return total
}

func (sh *StreamHandler) getPastData(flush bool) *StreamPastData {
	if len(sh.kernelSpans) == 0 && len(sh.allocations) == 0 {
		return nil
	}

	data := &StreamPastData{
		Spans:       sh.kernelSpans,
		Allocations: sh.allocations,
	}

	if flush {
		sh.kernelSpans = nil
		sh.allocations = nil
	}

	return data
}

func (sh *StreamHandler) getCurrentData(now uint64) *StreamCurrentData {
	if len(sh.kernelLaunches) == 0 && len(sh.memAllocEvents) == 0 {
		return nil
	}

	return &StreamCurrentData{
		Span:               sh.getCurrentKernelSpan(now),
		CurrentMemoryUsage: sh.getCurrentMemoryUsage(),
	}
}

func (sh *StreamHandler) markProcessEnded() error {
	nowTs, err := ddebpf.NowNanoseconds()
	if err != nil {
		return err
	}

	sh.processEnded = true
	sh.markSynchronization(uint64(nowTs))

	return nil
}
