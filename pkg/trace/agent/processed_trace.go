// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// ProcessedTrace represents a trace being processed in the agent.
type ProcessedTrace struct {
	Trace         pb.Trace
	WeightedTrace stats.WeightedTrace
	Root          *pb.Span
	Env           string
	Sublayers     stats.SublayerMap
}

// Weight returns the weight at the root span.
func (pt *ProcessedTrace) Weight() float64 {
	if pt.Root == nil {
		return 1.0
	}
	return stats.Weight(pt.Root)
}

// GetSamplingPriority returns the sampling priority of the root span.
func (pt *ProcessedTrace) GetSamplingPriority() (sampler.SamplingPriority, bool) {
	return sampler.GetSamplingPriority(pt.Root)
}
