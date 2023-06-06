// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpfcheck

import (
	"time"

	"github.com/cilium/ebpf"
)

// EBPFStats contains the statistics from the ebpf check
type EBPFStats struct {
	Maps        []EBPFMapStats
	PerfBuffers []EBPFPerfBufferStats
	Programs    []EBPFProgramStats
}

// EBPFMapStats are the basic statistics for ebpf maps
type EBPFMapStats struct {
	Name       string
	RSS        uint64
	MaxSize    uint64
	MaxEntries uint32
	Type       ebpf.MapType

	id uint32
}

// EBPFProgramStats are the basic statistics for ebpf programs
type EBPFProgramStats struct {
	Name            string
	Tag             string
	RSS             uint64
	RunCount        uint64
	RecursionMisses uint64
	XlatedProgLen   uint32
	VerifiedInsns   uint32
	Runtime         time.Duration
	Type            ebpf.ProgramType

	id uint32
}

// EBPFMmapStats is the detailed statistics for mmap-ed regions
type EBPFMmapStats struct {
	Addr uintptr
	Size uint64
	RSS  uint64
}

// EBPFCPUPerfBufferStats is the per-CPU statistics of a mmap region for a perf buffer
type EBPFCPUPerfBufferStats struct {
	EBPFMmapStats
	CPU uint32
}

// EBPFPerfBufferStats is the detailed statistics for a perf buffer
type EBPFPerfBufferStats struct {
	CPUBuffers []EBPFCPUPerfBufferStats
	EBPFMapStats
}
