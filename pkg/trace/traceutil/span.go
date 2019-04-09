package traceutil

import "github.com/StackVista/stackstate-agent/pkg/trace/pb"

const (
	// TraceMetricsKey is a tag key which, if set to true,
	// ensures all statistics are computed for this span.
	// [FIXME] *not implemented yet*
	TraceMetricsKey = "datadog.trace_metrics"

	// This is a special metric, it's 1 if the span is top-level, 0 if not.
	topLevelKey = "_top_level"
)

// HasTopLevel returns true if span is top-level.
func HasTopLevel(s *pb.Span) bool {
	return s.Metrics[topLevelKey] == 1
}

// HasForceMetrics returns true if statistics computation should be forced for this span.
func HasForceMetrics(s *pb.Span) bool {
	return s.Meta[TraceMetricsKey] == "true"
}

// SetTopLevel sets the top-level attribute of the span.
func SetTopLevel(s *pb.Span, topLevel bool) {
	if !topLevel {
		if s.Metrics == nil {
			return
		}
		delete(s.Metrics, topLevelKey)
		return
	}
	// Setting the metrics value, so that code downstream in the pipeline
	// can identify this as top-level without recomputing everything.
	setMetric(s, topLevelKey, 1)
}

func setMetric(s *pb.Span, key string, val float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
	s.Metrics[key] = val
}
