// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// singleSpanRateExtractor extracts spans that have been sampled using the single span sampling mechanism.
type singleSpanRateExtractor struct{}

func NewSingleSpanExtractor() Extractor {
	return &singleSpanRateExtractor{}
}

// Extract decides whether to extract a single span ingestion control event
// from the provided span having the specified priority. Extract returns a
// suggested extraction sample rate and a bool indicating whether an event was
// extracted. If the bool is false, then ignore the rate.
func (e *singleSpanRateExtractor) Extract(s *pb.Span, _ sampler.SamplingPriority) (float64, bool) {
	_, ok := traceutil.GetMetric(s, sampler.KeySpanSamplingMechanism)
	if ok {
		// If the tag is present, then the tracer wants us to keep the
		// span. The tracer already accounted for the rate.
		return 1, true
	}
	return 0, false
}
