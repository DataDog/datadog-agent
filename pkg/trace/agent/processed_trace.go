package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// ProcessedTrace represents a trace being processed in the agent.
type ProcessedTrace struct {
	Trace         pb.Trace
	WeightedTrace WeightedTrace
	Root          *pb.Span
	Env           string
	Sublayers     map[*pb.Span][]SublayerValue
	Sampled       bool
}

// Weight returns the weight at the root span.
func (pt *ProcessedTrace) Weight() float64 {
	if pt.Root == nil {
		return 1.0
	}
	return sampler.Weight(pt.Root)
}

// GetSamplingPriority returns the sampling priority of the root span.
func (pt *ProcessedTrace) GetSamplingPriority() (sampler.SamplingPriority, bool) {
	return sampler.GetSamplingPriority(pt.Root)
}
