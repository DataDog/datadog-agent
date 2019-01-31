package event

import (
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// Extractor extracts APM events from matching spans.
type Extractor interface {
	// Extract decides whether to extract an APM event from the provided span with the specified priority and returns
	// a suggested extraction sample rate and a bool value. If no event was extracted the bool value will be false and
	// the rate should not be used.
	Extract(span *stats.WeightedSpan, priority sampler.SamplingPriority) (rate float64, ok bool)
}
