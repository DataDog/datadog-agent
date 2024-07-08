// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

type StreamData struct {
	Key   StreamKey     `json:"key"`
	Spans []*KernelSpan `json:"spans"`
}

type GPUStats struct {
	PastKernelSpans    []StreamData              `json:"past_kernel_spans"`
	CurrentKernelSpans []StreamData `json:"current_kernel_spans"`
}
