// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strconv"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

const logStatExtractorName = "log_stat_extractor"

// logStatMetricPrefix is the shared prefix for all metrics emitted by LogStatExtractor.
const logStatMetricPrefix = "log." + logStatExtractorName + ".status."

// canonicalStatuses is the set of output status values after normalization.
// emergency, alert → critical; notice → info; warning → warn.
var canonicalStatuses = map[string]struct{}{
	"critical": {},
	"error":    {},
	"warn":     {},
	"info":     {},
	"debug":    {},
}

// logStatMetricNames pre-computes the full metric name for each canonical status.
var logStatMetricNames = map[string]string{
	"critical": logStatMetricPrefix + "critical.count",
	"error":    logStatMetricPrefix + "error.count",
	"warn":     logStatMetricPrefix + "warn.count",
	"info":     logStatMetricPrefix + "info.count",
	"debug":    logStatMetricPrefix + "debug.count",
}

// normalizeStatus maps raw log status strings to a canonical value.
// Groupings: emergency/alert/critical → "critical"; notice/info → "info";
// warning → "warn". Anything else falls back to "info".
func normalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "emergency", "alert", "critical":
		return "critical"
	case "error":
		return "error"
	case "warn", "warning":
		return "warn"
	case "notice", "info":
		return "info"
	case "debug":
		return "debug"
	default:
		return "info"
	}
}

// LogStatExtractor is a LogMetricsExtractor that counts logs per canonical
// status, grouped by the same four tag dimensions as the log pattern extractor
// (source, service, env, host).
type LogStatExtractor struct {
	contextByKey map[string]*observerdef.MetricContext
}

var _ observerdef.LogMetricsExtractor = (*LogStatExtractor)(nil)
var _ observerdef.ContextProvider = (*LogStatExtractor)(nil)

// NewLogStatExtractor creates a new LogStatExtractor.
func NewLogStatExtractor() *LogStatExtractor {
	return &LogStatExtractor{}
}

// Name returns "log_stat_extractor".
func (e *LogStatExtractor) Name() string {
	return logStatExtractorName
}

// ProcessLog normalizes the log status and emits a count metric per
// (status, tag-group) pair. Unknown statuses are counted under "info".
func (e *LogStatExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	status := normalizeStatus(log.GetStatus())

	groupTags := tagsForPatternGrouping(log.GetTags(), log.GetHostname())
	group := extractTagGroupByKey(groupTags)
	groupHash := tagGroupByKeyHash(group)

	contextKey := "status:" + status + "|" + strconv.FormatUint(groupHash, 16)

	if e.contextByKey == nil {
		e.contextByKey = make(map[string]*observerdef.MetricContext)
	}

	ctx, exists := e.contextByKey[contextKey]
	if !exists {
		ctx = &observerdef.MetricContext{
			Pattern:   status,
			Source:    logStatExtractorName,
			SplitTags: group.AsMap(),
		}
		e.contextByKey[contextKey] = ctx
	}
	ctx.Example = string(log.GetContent())

	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{
			{
				Name:       logStatMetricNames[status],
				Value:      1.0,
				Tags:       log.GetTags(),
				ContextKey: contextKey,
			},
		},
	}
}

// GetContextByKey implements observerdef.ContextProvider.
func (e *LogStatExtractor) GetContextByKey(key string) (observerdef.MetricContext, bool) {
	if e.contextByKey == nil {
		return observerdef.MetricContext{}, false
	}
	ctx, ok := e.contextByKey[key]
	if !ok {
		return observerdef.MetricContext{}, false
	}
	return *ctx, true
}
