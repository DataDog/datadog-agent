// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package topn

import (
	"slices"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// NewPerFlushFilter will create a per flush filter for the given config. This filter will reduce "n" into "k" rows per flush period.
func NewPerFlushFilter(n int64, flushConfig common.FlushConfig) *PerFlushFilter {
	return &PerFlushFilter{
		n:           n,
		flushConfig: flushConfig,
		scheduler:   newThrottler(n, flushConfig),
	}
}

// PerFlushFilter implements the FlowFlushFilter interface to return the top "k" flows for a given flush, where:
//
//	k * NumFlushes / CollectionPeriod = N
type PerFlushFilter struct {
	n           int64
	flushConfig common.FlushConfig
	scheduler   interface {
		GetNumRowsToFlushFor(ctx common.FlushContext) int
	}
}

// Filter implements the FlowFlushFilter interface to return the top "k" flows for a given flush. It will also adapt
// to cases where the FlushContext indicates that multiple periods are being flushed (this will occasionally happen;
// causes may be downtime, latencies, large CPU pauses, etc.)
func (p *PerFlushFilter) Filter(ctx common.FlushContext, flows []*common.Flow) []*common.Flow {
	numFlowsToPublish := p.scheduler.GetNumRowsToFlushFor(ctx)
	if numFlowsToPublish == 0 {
		return flows
	}

	if len(flows) <= numFlowsToPublish {
		return flows
	}

	slices.SortFunc(flows, reversed(compareByBytesAscending))

	return flows[:numFlowsToPublish]
}
