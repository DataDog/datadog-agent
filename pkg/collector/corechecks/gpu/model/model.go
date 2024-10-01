// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model contains the model for the GPU check, with types shared between the system-probe GPU probe and
// the gpu core agent check
package model

// MemoryAllocation represents a memory allocation event
type MemoryAllocation struct {
	// Start is the kernel-time timestamp of the allocation event
	StartKtime uint64 `json:"start"`

	// End is the kernel-time timestamp of the deallocation event. If 0, this means the allocation was not deallocated yet
	EndKtime uint64 `json:"end"`

	// Size is the size of the allocation in bytes
	Size uint64 `json:"size"`

	// IsLeaked is true if the allocation was not deallocated
	IsLeaked bool `json:"is_leaked"`
}

// KernelSpan represents a span of time during which one or more kernels were running on a GPU until
// a synchronization event happened
type KernelSpan struct {
	// StartKtime is the kernel-time timestamp of the start of the span, the moment the first kernel was launched
	StartKtime uint64 `json:"start"`

	// EndKtime is the kernel-time timestamp of the end of the span, the moment the synchronization event happened
	EndKtime uint64 `json:"end"`

	// AvgThreadCount is the average number of threads running on the GPU during the span
	AvgThreadCount uint64 `json:"avg_thread_count"`

	// NumKernels is the number of kernels that were launched during the span
	NumKernels uint64 `json:"num_kernels"`
}

// StreamKey is a unique identifier for a CUDA stream
type StreamKey struct {
	Pid    uint32 `json:"pid"`
	Stream uint64 `json:"stream"`
}

// StreamData contains kernel spans and allocations for a stream
type StreamData struct {
	Key         StreamKey           `json:"key"`
	Spans       []*KernelSpan       `json:"spans"`
	Allocations []*MemoryAllocation `json:"allocations"`
}

// GPUStats contains the past and current data for all streams, including kernel spans and allocations.
// This is the data structure that is sent to the agent
type GPUStats struct {
	// PastData contains the past kernel spans and allocations for all streams
	PastData []*StreamData `json:"past_kernel_spans"`

	// CurrentData contains currently active kernel spans and allocations for all streams
	CurrentData []*StreamData `json:"current_kernel_spans"`
}
