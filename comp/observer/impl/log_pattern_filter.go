// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/observer/def"

// minLogPatternRateLogsPerSec is the minimum log rate (logs/s) for a
// log_pattern_extractor anomaly to be forwarded to correlators and reporters.
// Anomalies on patterns that arrive slower than this threshold are suppressed
// because they are unlikely to represent a meaningful workload change.
const minLogPatternRateLogsPerSec = 1.0

// logPatternRateFilter suppresses anomalies produced by the log_pattern_extractor
// when the current metric rate (DebugInfo.CurrentValue, in logs/s) is below
// minLogPatternRateLogsPerSec.
//
// Rationale: patterns that fire less than once per second are too sparse to
// generate reliable anomaly signals — baseline estimation is noisy and false
// positives are common. The threshold can be adjusted via the constant above.
type logPatternRateFilter struct{}

var _ observerdef.DetectorFilter = (*logPatternRateFilter)(nil)

// NewLogPatternRateFilter returns a DetectorFilter that discards low-rate
// log_pattern_extractor anomalies (< 1 log/s at detection time).
func NewLogPatternRateFilter() observerdef.DetectorFilter {
	return &logPatternRateFilter{}
}

// defaultDetectorFilters returns the set of DetectorFilters applied in every
// engine instance (live observer and testbench replay).
func defaultDetectorFilters() []observerdef.DetectorFilter {
	return []observerdef.DetectorFilter{
		NewLogPatternRateFilter(),
	}
}

// Name returns the filter name.
func (f *logPatternRateFilter) Name() string {
	return "log_pattern_rate_filter"
}

// ShouldFilterOut returns true when the anomaly originates from the
// log_pattern_extractor namespace and its current rate is below 1 log/s.
// Anomalies that lack DebugInfo (no rate information) are passed through
// to avoid silently dropping detections from detectors that do not populate it.
func (f *logPatternRateFilter) ShouldFilterOut(a observerdef.Anomaly) bool {
	if a.Source.Namespace != LogPatternExtractorName {
		return false
	}
	if a.DebugInfo == nil {
		return false
	}
	return a.DebugInfo.CurrentValue < minLogPatternRateLogsPerSec
}
