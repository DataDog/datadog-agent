// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// defaultMinClusterSizeBeforeEmitMetrics is the minimum number of logs
// inside a cluster (pattern) before we emit a metric.
const defaultMinClusterSizeBeforeEmitMetrics = 5

// TODO(agent-q): Add a test to ensure this is >= the time we evict metrics
// defaultClusterTimeToLive is the time to live for a cluster.
// If a cluster hasn't been seen since this time, it will be removed.
const defaultClusterTimeToLive = 4 * time.Hour

const defaultGarbageCollectionInterval = 1 * time.Hour

// PatternKeyInfo contains what can identify a pattern.
type PatternKeyInfo struct {
	ClusterID int64
}

// NewPatternKeyInfo creates a PatternKeyInfo for the given cluster ID.
func NewPatternKeyInfo(clusterID int64) PatternKeyInfo {
	return PatternKeyInfo{ClusterID: clusterID}
}

// LogPatternExtractorConfig holds configuration for the LogPatternExtractor.
type LogPatternExtractorConfig struct{}

// DefaultLogPatternExtractorConfig returns a LogPatternExtractorConfig with default values.
func DefaultLogPatternExtractorConfig() LogPatternExtractorConfig {
	return LogPatternExtractorConfig{}
}

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	PatternClusterer          *patterns.PatternClusterer
	ctx                       logPatternExtractorContext
	NextGarbageCollectionTime int64
	// MinPatternsBeforeEmit is the minimum number of distinct patterns (clusters)
	// before emitting metrics. Zero means defaultMinPatternsBeforeEmitMetrics.
	MinPatternsBeforeEmit        int
	ClusterTimeToLiveSec         int
	GarbageCollectionIntervalSec int
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)
var _ observerdef.ContextProvider = (*LogPatternExtractor)(nil)

type patternMetricContext struct {
	keyInfo PatternKeyInfo
	example string
}

// logPatternExtractorContext holds per-metric context for GetContextByKey and
// indexes keys by cluster ID for O(cluster) deletion on GC.
type logPatternExtractorContext struct {
	byKey         map[string]patternMetricContext
	keysByCluster map[int64][]string
}

func (c *logPatternExtractorContext) get(key string) (patternMetricContext, bool) {
	if c.byKey == nil {
		return patternMetricContext{}, false
	}
	v, ok := c.byKey[key]
	return v, ok
}

func (c *logPatternExtractorContext) put(clusterID int64, contextKey string, entry patternMetricContext) {
	if c.byKey == nil {
		c.byKey = make(map[string]patternMetricContext)
	}
	if _, exists := c.byKey[contextKey]; !exists {
		if c.keysByCluster == nil {
			c.keysByCluster = make(map[int64][]string)
		}
		c.keysByCluster[clusterID] = append(c.keysByCluster[clusterID], contextKey)
	}
	c.byKey[contextKey] = entry
}

func (c *logPatternExtractorContext) removeCluster(clusterID int64) {
	if c.byKey == nil {
		return
	}
	for _, k := range c.keysByCluster[clusterID] {
		delete(c.byKey, k)
	}
	delete(c.keysByCluster, clusterID)
}

// NewLogPatternExtractor creates a new LogPatternExtractor.
func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
		MinPatternsBeforeEmit:        defaultMinClusterSizeBeforeEmitMetrics,
		ClusterTimeToLiveSec:         int(defaultClusterTimeToLive.Seconds()),
		GarbageCollectionIntervalSec: int(defaultGarbageCollectionInterval.Seconds()),
	}
}

// Name returns the extractor name.
func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

// Reset clears clustering and cached per-series context so reanalysis starts
// from the currently observed logs.
func (e *LogPatternExtractor) Reset() {
	e.PatternClusterer = patterns.NewPatternClusterer(patterns.IDComputeInfo{
		Offset: 0,
		Stride: 1,
		Index:  0,
	})
	e.ctx = logPatternExtractorContext{}
	e.NextGarbageCollectionTime = 0
}

// GetContextByKey implements observerdef.ContextProvider for pattern metrics
// emitted by this extractor.
func (e *LogPatternExtractor) GetContextByKey(key string) (observerdef.MetricContext, bool) {
	entry, ok := e.ctx.get(key)
	if !ok {
		return observerdef.MetricContext{}, false
	}

	pattern := ""
	cluster, err := e.PatternClusterer.GetCluster(entry.keyInfo.ClusterID)
	if err == nil && cluster != nil {
		pattern = cluster.PatternString()
	}

	return observerdef.MetricContext{
		Pattern: pattern,
		Example: entry.example,
		Source:  e.Name(),
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
	logUnixSec := log.GetTimestampUnixMilli() / 1000
	if logUnixSec == 0 {
		logUnixSec = time.Now().Unix()
	}
	gc := e.maybeGarbageCollect(logUnixSec)
	telemetry := []observerdef.ObserverTelemetry{}
	result := observerdef.LogMetricsExtractorOutput{
		EvictedContextKeys: gc.contextKeys,
	}
	if gc.clustersEvicted > 0 {
		// We count active patterns so we remove them
		telemetry = append(telemetry, newTelemetryCounter([]string{"detector:" + e.Name()}, telemetryLogPatternExtractorPatternCount, -float64(gc.clustersEvicted), logUnixSec))
	}
	if !logSeverityIsWarnPlus(log) {
		return result
	}
	message := string(log.GetContent())
	cluster, ok := e.PatternClusterer.Process(message, logUnixSec)
	if !ok {
		return result
	}
	// Not enough patterns yet, don't emit metric
	// It's not directly a new pattern but the first time we reach the threshold and we emit a metric
	if cluster.Count == e.MinPatternsBeforeEmit {
		telemetry = append(telemetry, newTelemetryCounter([]string{"detector:" + e.Name()}, telemetryLogPatternExtractorPatternCount, 1, logUnixSec))
	} else if cluster.Count < e.MinPatternsBeforeEmit {
		return result
	}

	patternKey := NewPatternKeyInfo(cluster.ID)
	metricName := fmt.Sprintf("log.%s.%x.count", e.Name(), cluster.ID+1)
	contextKey := metricContextKey(metricName, log.GetTags())

	e.ctx.put(cluster.ID, contextKey, patternMetricContext{
		keyInfo: patternKey,
		example: message,
	})

	result.Metrics = []observerdef.MetricOutput{{
		Name:       metricName,
		Value:      1,
		Tags:       log.GetTags(),
		ContextKey: contextKey,
	}}
	result.Telemetry = telemetry
	return result
}

// gcResult holds what was evicted during a garbage-collection pass.
type gcResult struct {
	contextKeys     []string
	clustersEvicted int
}

// maybeGarbageCollect removes stale clusters and returns the context keys and
// metric names evicted so the engine can drop contextRefs and storage series.
func (e *LogPatternExtractor) maybeGarbageCollect(currentTime int64) gcResult {
	if currentTime < e.NextGarbageCollectionTime {
		return gcResult{}
	}
	e.NextGarbageCollectionTime = currentTime + int64(e.GarbageCollectionIntervalSec)

	toDelete := e.PatternClusterer.ClusterIDsBeforeUnix(currentTime - int64(e.ClusterTimeToLiveSec))
	if len(toDelete) == 0 {
		return gcResult{}
	}
	var result gcResult
	for _, clusterID := range toDelete {
		if e.ctx.keysByCluster != nil {
			result.contextKeys = append(result.contextKeys, e.ctx.keysByCluster[clusterID]...)
		}
		e.ctx.removeCluster(clusterID)
	}
	result.clustersEvicted = len(toDelete)
	_ = e.PatternClusterer.RemoveClusters(toDelete)
	return result
}
