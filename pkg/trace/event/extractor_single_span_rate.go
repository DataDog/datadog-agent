// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// singleSpanRateExtractor extracts spans that have been sampled using the single span sampling mechanism.
type singleSpanRateExtractor struct{}

func NewSingleSpanExtractor() Extractor {
	return &singleSpanRateExtractor{}
}

func (e *singleSpanRateExtractor) Extract(s *pb.Span, _ sampler.SamplingPriority) (float64, bool) {
	m, ok := traceutil.GetMetric(s, sampler.KeySpanSamplingMechanism)
	if !ok || m != float64(apmsampling.SamplingMechanismSingleSpan) {
		return 0, false
	}
	extractionRate, ok := s.Metrics[sampler.KeySpanSamplingRuleRate]
	if !ok || extractionRate < 0 {
		return 0, false
	}
	if extractionRate > 0 {
		// If the trace has been manually sampled, we keep all matching spans
		extractionRate = 1
	}
	return extractionRate, true
}
