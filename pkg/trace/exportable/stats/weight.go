// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/traceutil"
)

// WeightedSpan extends Span to contain weights required by the Concentrator.
type WeightedSpan struct {
	Weight   float64 // Span weight. Similar to the trace root.Weight().
	TopLevel bool    // Is this span a service top-level or not. Similar to span.TopLevel().
	Measured bool    // Is this span marked for metrics computation.

	*pb.Span
}

// WeightedTrace is a slice of WeightedSpan pointers.
type WeightedTrace []*WeightedSpan

// NewWeightedTrace returns a weighted trace, with coefficient required by the concentrator.
func NewWeightedTrace(trace pb.Trace, root *pb.Span) WeightedTrace {
	wt := make(WeightedTrace, len(trace))

	weight := Weight(root)

	for i := range trace {
		wt[i] = &WeightedSpan{
			Span:     trace[i],
			Weight:   weight,
			TopLevel: traceutil.HasTopLevel(trace[i]),
			Measured: traceutil.IsMeasured(trace[i]),
		}
	}
	return wt
}

// keySamplingRateGlobal is a metric key holding the global sampling rate.
const keySamplingRateGlobal = "_sample_rate"

// Weight returns the weight of the span as defined for sampling, i.e. the
// inverse of the sampling rate.
func Weight(s *pb.Span) float64 {
	if s == nil {
		return 1.0
	}
	sampleRate, ok := s.Metrics[keySamplingRateGlobal]
	if !ok || sampleRate <= 0.0 || sampleRate > 1.0 {
		return 1.0
	}

	return 1.0 / sampleRate
}
