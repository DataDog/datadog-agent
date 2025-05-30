// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// keySamplingRateGlobal is a metric key holding the global sampling rate.
const keySamplingRateGlobal = "_sample_rate"

// This is a helper function to avoid duplicating the logic for calculating the weight.
func calculateWeight(sampleRate float64, ok bool) float64 {
	if !ok || sampleRate <= 0.0 || sampleRate > 1.0 {
		return 1
	}
	return 1.0 / sampleRate
}

// weight returns the weight of the span as defined for sampling, i.e. the
// inverse of the sampling rate.
func weight(s *pb.Span) float64 {
	if s == nil {
		return 1
	}
	sampleRate, ok := s.Metrics[keySamplingRateGlobal]
	return calculateWeight(sampleRate, ok)
}

// weightV1 returns the weight of the span as defined for sampling, i.e. the
// inverse of the sampling rate.
func weightV1(s *idx.InternalSpan) float64 {
	if s == nil {
		return 1
	}
	sampleRate, ok := s.GetAttributeAsFloat64(keySamplingRateGlobal)
	return calculateWeight(sampleRate, ok)
}
