// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"math"

	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
)

type StreamKey struct {
	Pid    uint32
	Tid    uint32
	Stream uint64
}

type StreamHandler struct {
	kernelLaunches []*gpuebpf.CudaKernelLaunch
	memEvents      []*gpuebpf.CudaMemEvent
	kernelSpans    []*KernelSpan
}

type KernelSpan struct {
	Start          uint64
	End            uint64
	AvgThreadCount uint64
}

func (sh *StreamHandler) handleKernelLaunch(event *gpuebpf.CudaKernelLaunch) {
	sh.kernelLaunches = append(sh.kernelLaunches, event)
}

func (sh *StreamHandler) handleMemEvent(event *gpuebpf.CudaMemEvent) {
	sh.memEvents = append(sh.memEvents, event)
}

func (sh *StreamHandler) handleSync(event *gpuebpf.CudaSync) {
	// TODO: Worry about concurrent calls to this?
	span := sh.getCurrentKernelSpan(event.Header.Ktime_ns)
	sh.kernelSpans = append(sh.kernelSpans, span)

	remainingLaunches := []*gpuebpf.CudaKernelLaunch{}
	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= span.End {
			remainingLaunches = append(remainingLaunches, launch)
		}
	}
	sh.kernelLaunches = remainingLaunches

}

func (sh *StreamHandler) getCurrentKernelSpan(maxTime uint64) *KernelSpan {
	span := KernelSpan{
		Start: math.MaxUint64,
		End:   0,
	}

	numLaunches := 0

	for _, launch := range sh.kernelLaunches {
		if launch.Header.Ktime_ns >= maxTime {
			continue
		}

		span.Start = min(launch.Header.Ktime_ns, span.Start)
		span.End = max(launch.Header.Ktime_ns, span.End)
		blockSize := launch.Block_size.X * launch.Block_size.Y * launch.Block_size.Z
		blockCount := launch.Grid_size.X * launch.Grid_size.Y * launch.Grid_size.Z
		span.AvgThreadCount += uint64(blockSize) * uint64(blockCount)
		numLaunches++
	}

	if numLaunches > 0 {
		span.AvgThreadCount /= uint64(numLaunches)
	}

	return &span
}
