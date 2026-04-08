// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// LogPatternExtractorName is the canonical name for the log pattern extractor.
// It is used as the storage namespace for emitted metrics, as the component
// name in the catalog, and in notify formatting for log-derived anomalies.
const LogPatternExtractorName = "log_pattern_extractor"

// LogPatternExtractorConfig holds hyperparameters for the log pattern extractor.
type LogPatternExtractorConfig struct {
	// MinClusterSizeBeforeEmit is the minimum number of logs matching a pattern
	// before emitting metrics. Zero means the default from DefaultLogPatternExtractorConfig.
	MinClusterSizeBeforeEmit int `json:"min_cluster_size_before_emit,omitempty"`
	// MaxTokenizedStringLength caps input length before tokenization (0 = patterns default).
	MaxTokenizedStringLength int `json:"max_tokenized_string_length,omitempty"`
	// MaxNumTokens caps token count per message (0 = patterns default).
	MaxNumTokens int `json:"max_num_tokens,omitempty"`
	// ParseHexDump controls hex-dump recognition in the tokenizer. When nil, the
	// patterns package default applies (true).
	ParseHexDump *bool `json:"parse_hex_dump,omitempty"`
	// MinTokenMatchRatio is the minimum fraction of token positions (by value)
	// that must match for two log lines to merge into one pattern. Range (0,1];
	// zero means the default 0.5 (Drain-style).
	MinTokenMatchRatio float64 `json:"min_token_match_ratio,omitempty"`
}

// DefaultLogPatternExtractorConfig returns defaults aligned with the patterns package.
func DefaultLogPatternExtractorConfig() LogPatternExtractorConfig {
	return LogPatternExtractorConfig{
		MinClusterSizeBeforeEmit: 5,
	}
}

func tokenizerFromConfig(cfg LogPatternExtractorConfig) *patterns.Tokenizer {
	t := patterns.NewTokenizer()
	if cfg.MaxTokenizedStringLength > 0 {
		t.MaxStringLen = cfg.MaxTokenizedStringLength
	}
	if cfg.MaxNumTokens > 0 {
		t.MaxTokens = cfg.MaxNumTokens
	}
	if cfg.ParseHexDump != nil {
		t.ParseHexDump = *cfg.ParseHexDump
	}
	return t
}

// TODO(agent-q): Add a test to ensure this is >= the time we evict metrics
// defaultClusterTimeToLive is the time to live for a cluster.
// If a cluster hasn't been seen since this time, it will be removed.
const defaultClusterTimeToLive = 4 * time.Hour

const defaultGarbageCollectionInterval = 1 * time.Hour

// PatternKeyInfo contains what can identify a pattern.
type PatternKeyInfo struct {
	ClusterID int64
	GroupHash uint64
}

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	taggedClusterer              *TaggedPatternClusterer
	registry                     *TagGroupByKeyRegistry
	ctx                          logPatternExtractorContext
	NextGarbageCollectionTime    int64
	ClusterTimeToLiveSec         int
	GarbageCollectionIntervalSec int
	// config is the resolved hyperparameters (MinClusterSizeBeforeEmit is never zero after init).
	config LogPatternExtractorConfig
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)
var _ observerdef.ContextProvider = (*LogPatternExtractor)(nil)

type patternMetricContext struct {
	keyInfo PatternKeyInfo
	example string
}

// logPatternExtractorContext holds per-metric context for GetContextByKey and
// indexes keys by tagged cluster (globalClusterHash) for O(cluster) deletion on GC.
// The tagged key encodes both groupHash and clusterID so that different sub-clusterers
// with coincidentally equal cluster IDs don't collide.
type logPatternExtractorContext struct {
	byKey               map[string]patternMetricContext
	keysByTaggedCluster map[string][]string // key: globalClusterHash(groupHash, clusterID)
}

func (c *logPatternExtractorContext) get(key string) (patternMetricContext, bool) {
	if c.byKey == nil {
		return patternMetricContext{}, false
	}
	v, ok := c.byKey[key]
	return v, ok
}

func (c *logPatternExtractorContext) put(groupHash uint64, clusterID int64, contextKey string, entry patternMetricContext) {
	taggedKey := globalClusterHash(groupHash, clusterID)
	if c.byKey == nil {
		c.byKey = make(map[string]patternMetricContext)
	}
	if _, exists := c.byKey[contextKey]; !exists {
		if c.keysByTaggedCluster == nil {
			c.keysByTaggedCluster = make(map[string][]string)
		}
		c.keysByTaggedCluster[taggedKey] = append(c.keysByTaggedCluster[taggedKey], contextKey)
		c.byKey[contextKey] = entry
	}
}

func (c *logPatternExtractorContext) removeTaggedCluster(taggedKey string) {
	if c.byKey == nil {
		return
	}
	for _, k := range c.keysByTaggedCluster[taggedKey] {
		delete(c.byKey, k)
	}
	delete(c.keysByTaggedCluster, taggedKey)
}

// NewLogPatternExtractor creates a new LogPatternExtractor.
// A zero-value cfg is accepted; zero fields fall back to DefaultLogPatternExtractorConfig values.
func NewLogPatternExtractor(cfg LogPatternExtractorConfig) *LogPatternExtractor {
	defaults := DefaultLogPatternExtractorConfig()
	if cfg.MinClusterSizeBeforeEmit <= 0 {
		cfg.MinClusterSizeBeforeEmit = defaults.MinClusterSizeBeforeEmit
	}
	registry := NewTagGroupByKeyRegistry()
	tok := tokenizerFromConfig(cfg)
	newSub := func() *patterns.PatternClusterer {
		return patterns.NewPatternClustererWithTokenizer(tok, cfg.MinTokenMatchRatio)
	}
	return &LogPatternExtractor{
		taggedClusterer:              NewTaggedPatternClustererWithFactory(registry, newSub),
		registry:                     registry,
		config:                       cfg,
		ClusterTimeToLiveSec:         int(defaultClusterTimeToLive.Seconds()),
		GarbageCollectionIntervalSec: int(defaultGarbageCollectionInterval.Seconds()),
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
	groupTags := tagsForPatternGrouping(log.GetTags(), log.GetHostname())
	groupHash, cluster, ok := e.taggedClusterer.Process(groupTags, message, logUnixSec)
	if !ok {
		return result
	}
	// Not enough patterns yet, don't emit metric
	// It's not directly a new pattern but the first time we reach the threshold and we emit a metric
	if cluster.Count == e.config.MinClusterSizeBeforeEmit {
		telemetry = append(telemetry, newTelemetryCounter([]string{"detector:" + e.Name()}, telemetryLogPatternExtractorPatternCount, 1, logUnixSec))
	} else if cluster.Count < e.config.MinClusterSizeBeforeEmit {
		return result
	}

	metricName := "log." + e.Name() + "." + globalClusterHash(groupHash, cluster.ID) + ".count"
	contextKey := metricContextKey(metricName, log.GetTags())

	e.ctx.put(groupHash, cluster.ID, contextKey, patternMetricContext{
		keyInfo: PatternKeyInfo{ClusterID: cluster.ID, GroupHash: groupHash},
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

// maybeGarbageCollect removes stale clusters from all sub-clusterers and
// returns the context keys evicted so the engine can drop matching contextRefs.
func (e *LogPatternExtractor) maybeGarbageCollect(currentTime int64) gcResult {
	if currentTime < e.NextGarbageCollectionTime {
		return gcResult{}
	}
	e.NextGarbageCollectionTime = currentTime + int64(e.GarbageCollectionIntervalSec)

	cutoff := currentTime - int64(e.ClusterTimeToLiveSec)
	evicted := e.taggedClusterer.GarbageCollectBefore(cutoff)
	if len(evicted) == 0 {
		return gcResult{}
	}
	var result gcResult
	for _, ev := range evicted {
		taggedKey := globalClusterHash(ev.GroupHash, ev.ClusterID)
		if e.ctx.keysByTaggedCluster != nil {
			result.contextKeys = append(result.contextKeys, e.ctx.keysByTaggedCluster[taggedKey]...)
		}
		e.ctx.removeTaggedCluster(taggedKey)
	}
	result.clustersEvicted = len(evicted)
	return result
}
