// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/observer/def"

// Suppress log_pattern_extractor anomalies below this log rate (logs/s)
const minLogPatternRateLogsPerSec = 1.0

// logPatternRateFilter filters out low-rate log_pattern_extractor anomalies
type logPatternRateFilter struct{}

var _ observerdef.DetectorFilter = (*logPatternRateFilter)(nil)

// NewLogPatternRateFilter creates a log_pattern rate-based filter
func NewLogPatternRateFilter() observerdef.DetectorFilter {
	return &logPatternRateFilter{}
}

func (f *logPatternRateFilter) Name() string {
	return "log_pattern_rate_filter"
}

// Filter out log_pattern_extractor anomalies with rate < 1 log/s. Pass through if no DebugInfo.
func (f *logPatternRateFilter) ShouldFilterOut(a observerdef.Anomaly) bool {
	if a.Source.Namespace != LogPatternExtractorName {
		return false
	}
	if a.DebugInfo == nil {
		return false
	}
	return a.DebugInfo.CurrentValue < minLogPatternRateLogsPerSec
}
