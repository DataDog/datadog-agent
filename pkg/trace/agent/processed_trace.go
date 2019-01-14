package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

type ProcessedTrace struct {
	Trace         pb.Trace
	WeightedTrace WeightedTrace
	Root          *pb.Span
	Env           string
	Sublayers     map[*pb.Span][]SublayerValue
	Sampled       bool
}

func (pt *ProcessedTrace) Weight() float64 {
	if pt.Root == nil {
		return 1.0
	}
	return sampler.Weight(pt.Root)
}

func (pt *ProcessedTrace) GetSamplingPriority() (sampler.SamplingPriority, bool) {
	return sampler.GetSamplingPriority(pt.Root)
}
