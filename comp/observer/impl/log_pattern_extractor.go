// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// defaultMinClusterSizeBeforeEmitMetrics is the minimum number of logs
// inside a cluster (pattern) before we emit a metric.
const defaultMinClusterSizeBeforeEmitMetrics = 5

// PatternKeyInfo contains what can identify a pattern.
type PatternKeyInfo struct {
	ClusterID int64
	GroupHash uint64
}

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	taggedClusterer *TaggedPatternClusterer
	registry        *TagGroupByKeyRegistry
	patternContext  map[string]patternMetricContext
	// MinPatternsBeforeEmit is the minimum number of distinct patterns (clusters)
	// before emitting metrics. Zero means defaultMinPatternsBeforeEmitMetrics.
	MinPatternsBeforeEmit int
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)
var _ observerdef.ContextProvider = (*LogPatternExtractor)(nil)

type patternMetricContext struct {
	keyInfo PatternKeyInfo
	example string
}

// NewLogPatternExtractor creates a new LogPatternExtractor.
func NewLogPatternExtractor() *LogPatternExtractor {
	registry := NewTagGroupByKeyRegistry()
	return &LogPatternExtractor{
		taggedClusterer:       NewTaggedPatternClusterer(registry),
		registry:              registry,
		MinPatternsBeforeEmit: defaultMinClusterSizeBeforeEmitMetrics,
	}
}

// Name returns the extractor name.
func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

// Reset clears clustering and cached per-series context so reanalysis starts
// from the currently observed logs. The registry is kept so that previously
// registered hashes remain resolvable.
func (e *LogPatternExtractor) Reset() {
	e.taggedClusterer.Reset()
	e.patternContext = nil
}

// GetContextByKey implements observerdef.ContextProvider for pattern metrics
// emitted by this extractor.
func (e *LogPatternExtractor) GetContextByKey(key string) (observerdef.MetricContext, bool) {
	if e.patternContext == nil {
		return observerdef.MetricContext{}, false
	}
	entry, ok := e.patternContext[key]
	if !ok {
		return observerdef.MetricContext{}, false
	}

	pattern := ""
	cluster, err := e.taggedClusterer.GetCluster(entry.keyInfo.GroupHash, entry.keyInfo.ClusterID)
	if err == nil && cluster != nil {
		pattern = cluster.PatternString()
	}

	group, _ := e.registry.Lookup(entry.keyInfo.GroupHash)
	return observerdef.MetricContext{
		Pattern:   pattern,
		Example:   entry.example,
		Source:    e.Name(),
		SplitTags: group.AsMap(),
	}, true
}

// logSeverityIsWarnPlus returns true when the log should be clustered: warning
func logSeverityIsWarnPlus(log observerdef.LogView) bool {
	status := strings.ToLower(strings.TrimSpace(log.GetStatus()))
	switch status {
	case "warn", "warning", "error", "critical", "fatal", "alert", "emergency":
		return true
	default:
		return false
	}
}

// ProcessLog clusters the log message and emits a count metric for its pattern.
func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	if !logSeverityIsWarnPlus(log) {
		return observerdef.LogMetricsExtractorOutput{}
	}
	telemetry := []observerdef.ObserverTelemetry{}
	message := string(log.GetContent())
	groupTags := tagsForPatternGrouping(log.GetTags(), log.GetHostname())
	groupHash, cluster, ok := e.taggedClusterer.Process(groupTags, message)
	if !ok {
		return observerdef.LogMetricsExtractorOutput{}
	}
	// Not enough patterns yet, don't emit metric.
	// It's not directly a new pattern but the first time we reach the threshold and we emit a metric.
	if cluster.Count == e.MinPatternsBeforeEmit {
		telemetry = append(telemetry, newTelemetryCounter([]string{"detector:" + e.Name()}, telemetryLogPatternExtractorPatternCount, 1, log.GetTimestampUnixMilli()/1000))
	} else if cluster.Count < e.MinPatternsBeforeEmit {
		return observerdef.LogMetricsExtractorOutput{}
	}

	metricName := "log." + e.Name() + "." + globalClusterHash(groupHash, cluster.ID) + ".count"
	contextKey := metricContextKey(metricName, log.GetTags())

	if e.patternContext == nil {
		e.patternContext = make(map[string]patternMetricContext)
	}
	if _, exists := e.patternContext[contextKey]; !exists {
		e.patternContext[contextKey] = patternMetricContext{
			keyInfo: PatternKeyInfo{ClusterID: cluster.ID, GroupHash: groupHash},
			example: message,
		}
	}

	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{
			Name:       metricName,
			Value:      1,
			Tags:       log.GetTags(),
			ContextKey: contextKey,
		}},
		Telemetry: telemetry,
	}
}
