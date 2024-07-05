// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"math"
)

type streamKey struct {
	pid    uint32
	tid    uint32
	stream uint64
}

type streamHandler struct {
	kernelLaunches []*CudaKernelLaunch
	memEvents      []*CudaMemEvent
	kernelSpans    []*kernelSpan
}

type kernelSpan struct {
	start          uint64
	end            uint64
	avgThreadCount uint64
}

func (sh *streamHandler) handleKernelLaunch(event *CudaKernelLaunch) {
	sh.kernelLaunches = append(sh.kernelLaunches, event)
}

func (sh *streamHandler) handleMemEvent(event *CudaMemEvent) {
	sh.memEvents = append(sh.memEvents, event)
}

func (sh *streamHandler) handleSync(event *CudaSync) {
	// TODO: Worry about concurrent calls to this?
	span := sh.getCurrentKernelSpan(event.Header.Ktime_ns)
	sh.kernelSpans = append(sh.kernelSpans, span)

	remainingLaunches := []*CudaKernelLaunch{}
	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= span.end {
			remainingLaunches = append(remainingLaunches, launch)
		}
	}
	sh.kernelLaunches = remainingLaunches

}

func (sh *streamHandler) getCurrentKernelSpan(maxTime uint64) *kernelSpan {
	span := kernelSpan{
		start: math.MaxUint64,
		end:   0,
	}

	numLaunches := 0

	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= maxTime {
			continue
		}

		span.start = min(launch.Header.Ktime_ns, span.start)
		span.end = max(launch.Header.Ktime_ns, span.end)
		blockSize := launch.Block_size.X * launch.Block_size.Y * launch.Block_size.Z
		blockCount := launch.Grid_size.X * launch.Grid_size.Y * launch.Grid_size.Z
		span.avgThreadCount += uint64(blockSize) * uint64(blockCount)
		numLaunches++
	}

	if numLaunches > 0 {
		span.avgThreadCount /= uint64(numLaunches)
	}

	return &span
}
