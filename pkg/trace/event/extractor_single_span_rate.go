// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// singleSpanRateExtractor is a single span extractor that decides whether to extract spans from trace chunks based on
// availability of single span sampling tags, e.g. KeySpanSamplingMechanism.
type singleSpanRateExtractor struct{}

func NewSingleSpanExtractor() Extractor {
	return &singleSpanRateExtractor{}
}

func (e *singleSpanRateExtractor) Extract(s *pb.Span, _ sampler.SamplingPriority) (float64, bool) {
	if len(s.Metrics) == 0 {
		// metric not set
		return 0, false
	}
	m, ok := s.Metrics[sampler.KeySpanSamplingMechanism]
	if !ok || m != float64(apmsampling.SpanSamplingRuleMechanism) {
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
