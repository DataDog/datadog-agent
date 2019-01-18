package event

import (
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// noopExtractor is a no-op APM event extractor used when APM event extraction is disabled.
type noopExtractor struct{}

// NewNoopExtractor returns a new APM event extractor that does not extract any events.
func NewNoopExtractor() Extractor {
	return &noopExtractor{}
}

func (e *noopExtractor) Extract(_ *agent.WeightedSpan, _ sampler.SamplingPriority) (float64, bool) {
	return 0, false
}
