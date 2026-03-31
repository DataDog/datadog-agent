// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/observer/def"

// logPatternRateWindowSec is the window over which log rate is averaged.
const logPatternRateWindowSec = 60

// minLogPatternRateLogsPerSec is the minimum average log rate (logs/s) over the
// last logPatternRateWindowSec for a log_pattern_extractor anomaly to be
// forwarded. Anomalies on patterns averaging below this rate are suppressed
// because they are too sparse to produce actionable signals.
//
// Implementation note: log_pattern_extractor emits Value=1 per log, so
// AggregateAverage is always 1.0 (constant series — BOCPD never fires on it).
// Anomalies only fire on the AggregateCount series where each point equals the
// number of logs in that 1-second bucket. Summing the last 60 points and
// dividing by the window gives the average log rate in logs/s.
const minLogPatternRateLogsPerSec = 1.0

// logPatternRateFilter suppresses anomalies produced by the log_pattern_extractor
// when the average log rate over the last logPatternRateWindowSec is below
// minLogPatternRateLogsPerSec.
type logPatternRateFilter struct{}

var _ observerdef.DetectorFilter = (*logPatternRateFilter)(nil)

// NewLogPatternRateFilter returns a DetectorFilter that discards low-rate
// log_pattern_extractor anomalies (average < 1 log/s over the last 60 s).
func NewLogPatternRateFilter() observerdef.DetectorFilter {
	return &logPatternRateFilter{}
}

// Name returns the filter name.
func (f *logPatternRateFilter) Name() string {
	return "log_pattern_rate_filter"
}

// ShouldFilterOut returns true when the anomaly originates from the
// log_pattern_extractor namespace and the average AggregateCount rate over the
// last logPatternRateWindowSec is below minLogPatternRateLogsPerSec.
//
// When SourceRef is nil (no storage-backed series) the filter falls back to
// DebugInfo.CurrentValue so anomalies from detectors that do not populate
// SourceRef are never silently dropped.
func (f *logPatternRateFilter) ShouldFilterOut(a observerdef.Anomaly, storage observerdef.StorageReader, dataTime int64) bool {
	if a.Source.Namespace != LogPatternExtractorName {
		return false
	}

	if a.SourceRef != nil {
		totalLogs := storage.SumRange(a.SourceRef.Ref, dataTime-logPatternRateWindowSec, dataTime, observerdef.AggregateCount)
		return totalLogs/logPatternRateWindowSec < minLogPatternRateLogsPerSec
	}

	// Fallback: no storage ref available.
	if a.DebugInfo == nil {
		return false
	}
	return a.DebugInfo.CurrentValue < minLogPatternRateLogsPerSec
}
