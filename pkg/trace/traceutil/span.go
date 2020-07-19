// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traceutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traces"
)

const (
	// This is a special metric, it's 1 if the span is top-level, 0 if not.
	topLevelKey = "_top_level"

	// measuredKey is a special metric flag that marks a span for trace metrics calculation.
	measuredKey = "_dd.measured"
)

// HasTopLevel returns true if span is top-level.
func HasTopLevel(s traces.Span) bool {
	v, ok := s.GetMetric(topLevelKey)
	return ok && v == 1
}

// IsMeasured returns true if a span should be measured (i.e., it should get trace metrics calculated).
func IsMeasured(s traces.Span) bool {
	v, ok := s.GetMetric(measuredKey)
	return ok && v == 1
}

// SetTopLevel sets the top-level attribute of the span.
func SetTopLevel(s traces.Span, topLevel bool) {
	if !topLevel {
		// TODO: Actually delete.
		// delete(s.Metrics, topLevelKey)
		return
	}

	// Setting the metrics value, so that code downstream in the pipeline
	// can identify this as top-level without recomputing everything.
	s.SetMetric(topLevelKey, 1)
}

// GetMeta gets the metadata value in the span Meta map.
func GetMeta(s *pb.Span, key string) (string, bool) {
	if s.Meta == nil {
		return "", false
	}
	val, ok := s.Meta[key]
	return val, ok
}
