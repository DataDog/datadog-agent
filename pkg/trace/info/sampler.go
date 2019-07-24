// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package info

import "github.com/DataDog/datadog-agent/pkg/trace/sampler"

// SamplerInfo represents internal stats and state of a sampler
type SamplerInfo struct {
	// Stats contains statistics about what the sampler is doing.
	Stats SamplerStats
	// State is the internal state of the sampler (for debugging mostly)
	State sampler.InternalState
}

// SamplerStats contains sampler statistics
type SamplerStats struct {
	// KeptTPS is the number of traces kept (average per second for last flush)
	KeptTPS float64
	// TotalTPS is the total number of traces (average per second for last flush)
	TotalTPS float64
}
