// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package topn defines business logic for filtering NetFlow records to the Top "N" occurrences.
package topn

import (
	"slices"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// NewPerFlushFilter will create a per flush filter for the given config. This filter will reduce "n" into "k" rows per flush period.
func NewPerFlushFilter(n int64, flushConfig common.FlushConfig, sender sender.Sender, logger log.Component) *PerFlushFilter {
	return &PerFlushFilter{
		n:           n,
		flushConfig: flushConfig,
		throttler:   newThrottler(n, flushConfig, logger),
		metrics:     sender,
	}
}

// PerFlushFilter implements the FlowFlushFilter interface to return the top "k" flows for a given flush, where:
//
//	k * NumFlushes / CollectionPeriod = N
type PerFlushFilter struct {
	n           int64
	flushConfig common.FlushConfig
	throttler   interface {
		GetNumRowsToFlushFor(ctx common.FlushContext) int
	}
	metrics sender.Sender
}

// Filter implements the FlowFlushFilter interface to return the top "k" flows for a given flush. It will also adapt
// to cases where the FlushContext indicates that multiple periods are being flushed (this will occasionally happen;
// causes may be downtime, latencies, large CPU pauses, etc.)
func (p *PerFlushFilter) Filter(ctx common.FlushContext, flows []*common.Flow) []*common.Flow {
	start := time.Now()

	filteredFlows := p.applyFilters(ctx, flows)

	var bytesInDroppedFlows uint64
	var packetsInDroppedFlows uint64
	for _, flow := range filteredFlows.toDrop {
		bytesInDroppedFlows += flow.Bytes
		packetsInDroppedFlows += flow.Packets
	}

	p.metrics.Histogram("datadog.netflow.flow_truncation.runtime_ms", float64(time.Since(start).Milliseconds()), "", nil)
	p.metrics.Count("datadog.netflow.flow_truncation.flows_total", float64(len(flows)), "", nil)
	p.metrics.Count("datadog.netflow.flow_truncation.flows_kept", float64(len(filteredFlows.toPublish)), "", nil)
	p.metrics.Count("datadog.netflow.flow_truncation.flows_dropped.count", float64(len(filteredFlows.toDrop)), "", nil)
	p.metrics.Count("datadog.netflow.flow_truncation.flows_dropped.bytes", float64(bytesInDroppedFlows), "", nil)
	p.metrics.Count("datadog.netflow.flow_truncation.flows_dropped.packets", float64(packetsInDroppedFlows), "", nil)
	p.metrics.Gauge("datadog.netflow.flow_truncation.threshold_value", float64(p.n), "", nil)

	return filteredFlows.toPublish
}

func (p *PerFlushFilter) applyFilters(ctx common.FlushContext, flows []*common.Flow) filterResult {
	numFlowsToPublish := p.throttler.GetNumRowsToFlushFor(ctx)
	if numFlowsToPublish == 0 {
		return filterResult{
			toPublish: flows,
		}
	}

	if len(flows) <= numFlowsToPublish {
		return filterResult{
			toPublish: flows,
		}
	}

	slices.SortFunc(flows, reversed(compareByBytesAscending))

	return filterResult{
		toPublish: flows[:numFlowsToPublish],
		toDrop:    flows[numFlowsToPublish:],
	}
}

type filterResult struct {
	toPublish []*common.Flow
	toDrop    []*common.Flow
}
