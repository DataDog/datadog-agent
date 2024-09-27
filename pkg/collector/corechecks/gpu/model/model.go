// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This package contains the model for the GPU check, with types shared between the system-probe GPU probe and
// the gpu core agent check
package model

// MemoryAllocation represents a memory allocation event
type MemoryAllocation struct {
	// Start is the UNIX timestamp of the allocation event
	Start uint64 `json:"start"`

	// End is the UNIX timestamp of the deallocation event
	End uint64 `json:"end"`

	// Size is the size of the allocation in bytes
	Size uint64 `json:"size"`

	// IsLeaked is true if the allocation was not deallocated
	IsLeaked bool `json:"is_leaked"`
}

// KernelSpan represents a span of time during which one or more kernels were running on a GPU until
// a synchronization event happened
type KernelSpan struct {
	Start          uint64 `json:"start"`
	End            uint64 `json:"end"`
	AvgThreadCount uint64 `json:"avg_thread_count"`
	NumKernels     uint64 `json:"num_kernels"`
}

// StreamKey is a unique identifier for a CUDA stream
type StreamKey struct {
	Pid    uint32 `json:"pid"`
	Tid    uint32 `json:"tid"`
	Stream uint64 `json:"stream"`
}

// StreamPastData contains kernel spans and allocations that are no longer active
type StreamPastData struct {
	Key         StreamKey           `json:"key"`
	Spans       []*KernelSpan       `json:"spans"`
	Allocations []*MemoryAllocation `json:"allocations"`
}

// StreamCurrentData contains the current kernel span and allocations for a stream.
type StreamCurrentData struct {
	Key                StreamKey           `json:"key"`
	Span               *KernelSpan         `json:"span"`
	CurrentMemoryUsage uint64              `json:"current_memory_usage"`
	CurrentAllocations []*MemoryAllocation `json:"current_allocations"`
}

// GPUStats contains the past and current data for all streams
type GPUStats struct {
	PastData    []*StreamPastData    `json:"past_kernel_spans"`
	CurrentData []*StreamCurrentData `json:"current_kernel_spans"`
}
