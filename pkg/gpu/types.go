// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

type StreamPastData struct {
	Key         StreamKey           `json:"key"`
	Spans       []*KernelSpan       `json:"spans"`
	Allocations []*MemoryAllocation `json:"allocations"`
}

type StreamCurrentData struct {
	Key                StreamKey           `json:"key"`
	Span               *KernelSpan         `json:"span"`
	CurrentMemoryUsage uint64              `json:"current_memory_usage"`
	CurrentAllocations []*MemoryAllocation `json:"current_allocations"`
}

type GPUStats struct {
	PastData    []*StreamPastData    `json:"past_kernel_spans"`
	CurrentData []*StreamCurrentData `json:"current_kernel_spans"`
}
