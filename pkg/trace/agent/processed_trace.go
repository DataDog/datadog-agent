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
