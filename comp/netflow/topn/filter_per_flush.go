package topn

import (
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func NewPerFlushFilter(n int64, flushConfig common.FlushConfig) *PerFlushFilter {
	return &PerFlushFilter{
		n:           n,
		flushConfig: flushConfig,
		scheduler:   newScheduler(n, flushConfig),
	}
}

type PerFlushFilter struct {
	n           int64
	flushConfig common.FlushConfig
	scheduler   interface {
		GetNumRowsToFlushFor(ctx common.FlushContext) int
	}
}

type FlushConfig struct {
	FlowCollectionDuration time.Duration
	FlushTickFrequency     time.Duration
}

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
