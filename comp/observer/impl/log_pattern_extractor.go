// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// defaultMinClusterSizeBeforeEmitMetrics is the minimum number of logs
// inside a cluster (pattern) before we emit a metric.
const defaultMinClusterSizeBeforeEmitMetrics = 5

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
	PatternClusterer *patterns.PatternClusterer
	patternContext   map[string]patternMetricContext
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
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
		MinPatternsBeforeEmit: defaultMinClusterSizeBeforeEmitMetrics,
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

// TagGroupByKey holds the resolved values for the split tag dimensions.
// Absent tags (e.g. a log with no "env" tag) are represented by an empty string.
type TagGroupByKey struct {
	Source  string
	Service string
	Env     string
	Host    string
}

// AsMap returns a map of non-empty tag key→value pairs for this group.
func (c TagGroupByKey) AsMap() map[string]string {
	m := make(map[string]string, 4)
	if c.Source != "" {
		m["source"] = c.Source
	}
	if c.Service != "" {
		m["service"] = c.Service
	}
	if c.Env != "" {
		m["env"] = c.Env
	}
	if c.Host != "" {
		m["host"] = c.Host
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// tagGroupByKeyHash computes a stable fnv64a hash for a TagGroupByKey.
func tagGroupByKeyHash(c TagGroupByKey) uint64 {
	h := fnv.New64a()
	// Avoid allocating via Sprintf by writing directly to h
	h.Write([]byte(c.Source))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Service))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Env))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Host))
	return h.Sum64()
}

// TagGroupByKeyRegistry is a bidirectional, append-only store between a uint64 hash
// and a TagGroupByKey. It is safe for concurrent use.
type TagGroupByKeyRegistry struct {
	mu     sync.RWMutex
	byHash map[uint64]TagGroupByKey
}

// NewTagGroupByKeyRegistry creates an empty TagGroupByKeyRegistry.
func NewTagGroupByKeyRegistry() *TagGroupByKeyRegistry {
	return &TagGroupByKeyRegistry{byHash: make(map[uint64]TagGroupByKey)}
}

// Register inserts (or confirms) a TagGroupByKey and returns its stable hash.
// Calling Register twice with the same group returns the same hash.
func (r *TagGroupByKeyRegistry) Register(group TagGroupByKey) uint64 {
	hash := tagGroupByKeyHash(group)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byHash[hash]; !exists {
		r.byHash[hash] = group
	}
	return hash
}

// Lookup returns the TagGroupByKey for the given hash, and whether it was found.
func (r *TagGroupByKeyRegistry) Lookup(hash uint64) (TagGroupByKey, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	group, ok := r.byHash[hash]
	return group, ok
}

// ProcessLog clusters the log message and emits a count metric for its pattern.
func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	if !logSeverityIsWarnPlus(log) {
		return observerdef.LogMetricsExtractorOutput{}
	}
	telemetry := []observerdef.ObserverTelemetry{}
	message := string(log.GetContent())
	cluster, ok := e.PatternClusterer.Process(message)
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

	patternKey := NewPatternKeyInfo(cluster.ID)
	metricName := fmt.Sprintf("log.%s.%x.count", e.Name(), cluster.ID+1)
	contextKey := metricContextKey(metricName, log.GetTags())

	if e.patternContext == nil {
		e.patternContext = make(map[string]patternMetricContext)
	}
	e.patternContext[contextKey] = patternMetricContext{
		keyInfo: patternKey,
		example: message,
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
