package event

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// fallbackExtractor is an event extractor that always decides to extract APM events based on a fixed sample rate
type fallbackExtractor struct {
	defaultRate float64
}

// NewFixedRateExtractor returns an APM event extractor that always decides to extract APM events based on a fixed
// sample rate
func NewFallbackExtractor(defaultRate float64) Extractor {
	return &fallbackExtractor{defaultRate: defaultRate}
}

// Extract always returns the default extraction rate and a true value.
func (e *fallbackExtractor) Extract(s *pb.Span, priority sampler.SamplingPriority) (float64, bool) {
	return e.defaultRate, true
}
