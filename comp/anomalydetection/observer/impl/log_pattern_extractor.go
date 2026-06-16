// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/patterns"
)

// LogPatternExtractorName is the canonical name for the log pattern extractor.
// It is used as the storage namespace for emitted metrics, as the component
// name in the catalog, and in notify formatting for log-derived anomalies.
const LogPatternExtractorName = "log_pattern_extractor"

// TODO(agent-q): Add a test to ensure this is >= the time we evict metrics
// defaultClusterTimeToLive is the time to live for a cluster.
// If a cluster hasn't been seen since this time, it will be removed.
const defaultClusterTimeToLive = 4 * time.Hour

const defaultGarbageCollectionInterval = 1 * time.Hour

// LogPatternExtractorConfig holds hyperparameters for the log pattern extractor.
type LogPatternExtractorConfig struct {
	// This will disable all optimizations like MinClusterSizeBeforeEmit, ClusterTimeToLiveSec, etc.
	DisableOptimizations bool `json:"disable_optimizations,omitempty"`
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
	// ClusterTimeToLiveSec is how long (seconds) a cluster may go without a matching log before it is removed.
	// Zero disables cluster garbage collection.
	ClusterTimeToLiveSec int64 `json:"cluster_time_to_live_sec,omitempty"`
	// GarbageCollectionIntervalSec is the minimum time between GC passes when ClusterTimeToLiveSec > 0.
	GarbageCollectionIntervalSec int64 `json:"garbage_collection_interval_sec,omitempty"`
	// MaxPatternsPerGroup caps the number of live clusters in any single tag
	// group. When exceeded, the least-recently-seen cluster is evicted (LRU)
	// and its engine context is dropped. Zero means use the default; set
	// negative to disable. Bounds memory/series-cardinality on workloads with
	// high pattern diversity (e.g. container log churn).
	MaxPatternsPerGroup int `json:"max_patterns_per_group,omitempty"`
	// MaxTagGroups caps the number of distinct tag groups (source/service/env/host
	// combinations) tracked simultaneously. When exceeded, the least-recently-
	// touched group's clusters are all evicted at once. Zero means use the
	// default; set negative to disable.
	MaxTagGroups int `json:"max_tag_groups,omitempty"`
}

// DefaultLogPatternExtractorConfig returns defaults aligned with the patterns package.
func DefaultLogPatternExtractorConfig() LogPatternExtractorConfig {
	parseHexDump := true

	return LogPatternExtractorConfig{
		MinClusterSizeBeforeEmit:     5,
		ClusterTimeToLiveSec:         int64(defaultClusterTimeToLive.Seconds()),
		GarbageCollectionIntervalSec: int64(defaultGarbageCollectionInterval.Seconds()),
		MinTokenMatchRatio:           0.5,
		MaxTokenizedStringLength:     12500,
		MaxNumTokens:                 250,
		ParseHexDump:                 &parseHexDump,
		MaxPatternsPerGroup:          1024,
		MaxTagGroups:                 256,
	}
}

func (c *LogPatternExtractorConfig) RefreshConfig() {
	if c.DisableOptimizations {
		c.MinClusterSizeBeforeEmit = 0
		c.ClusterTimeToLiveSec = 0
		c.GarbageCollectionIntervalSec = 0
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

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	taggedClusterer           *TaggedPatternClusterer
	registry                  *TagGroupByKeyRegistry
	NextGarbageCollectionTime int64
	// config is the resolved hyperparameters (MinClusterSizeBeforeEmit is never zero after init).
	config    LogPatternExtractorConfig
	telemetry *observerTelemetry
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)

// NewLogPatternExtractor creates a new LogPatternExtractor.
// A zero-value cfg is accepted; zero fields fall back to DefaultLogPatternExtractorConfig values.
// MaxPatternsPerGroup and MaxTagGroups follow the same convention: 0 → default,
// negative → disabled (unbounded).
func NewLogPatternExtractor(cfg LogPatternExtractorConfig) *LogPatternExtractor {
	// Apply defaults first and then refresh config to finalize it
	defaults := DefaultLogPatternExtractorConfig()
	if cfg.MinClusterSizeBeforeEmit <= 0 {
		cfg.MinClusterSizeBeforeEmit = defaults.MinClusterSizeBeforeEmit
	}
	if !cfg.DisableOptimizations {
		if cfg.ClusterTimeToLiveSec <= 0 {
			cfg.ClusterTimeToLiveSec = defaults.ClusterTimeToLiveSec
		}
		if cfg.GarbageCollectionIntervalSec <= 0 {
			cfg.GarbageCollectionIntervalSec = defaults.GarbageCollectionIntervalSec
		}
	}
	if cfg.MaxPatternsPerGroup == 0 {
		cfg.MaxPatternsPerGroup = defaults.MaxPatternsPerGroup
	}
	if cfg.MaxTagGroups == 0 {
		cfg.MaxTagGroups = defaults.MaxTagGroups
	}
	cfg.RefreshConfig()

	registry := NewTagGroupByKeyRegistry()
	tok := tokenizerFromConfig(cfg)
	newSub := func() *patterns.PatternClusterer {
		return patterns.NewPatternClustererWithTokenizer(tok, cfg.MinTokenMatchRatio)
	}
	tc := NewTaggedPatternClustererWithFactory(registry, newSub)
	if cfg.MaxPatternsPerGroup > 0 {
		tc.MaxClustersPerGroup = cfg.MaxPatternsPerGroup
	}
	if cfg.MaxTagGroups > 0 {
		tc.MaxTagGroups = cfg.MaxTagGroups
	}
	return &LogPatternExtractor{
		taggedClusterer: tc,
		registry:        registry,
		config:          cfg,
	}
}

// Name returns the extractor name.
func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

// Reset clears clustering state so reanalysis starts from the currently
// observed logs. The registry is kept so that previously registered hashes
// remain resolvable.
func (e *LogPatternExtractor) Reset() {
	e.taggedClusterer.Reset()
	e.NextGarbageCollectionTime = 0
}

// SetObserverTelemetry allows wiring direct telemetry emission without
// transporting telemetry through extractor outputs.
func (e *LogPatternExtractor) SetObserverTelemetry(t *observerTelemetry) {
	e.telemetry = t
}

// ProcessLog clusters the log message and emits a count metric for its pattern.
func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	logUnixSec := log.GetTimestampUnixMilli() / 1000
	if logUnixSec == 0 {
		logUnixSec = time.Now().Unix()
	}
	gc := e.maybeGarbageCollect(logUnixSec)
	result := observerdef.LogMetricsExtractorOutput{
		EvictedMetricNames: gc.metricNames,
	}
	if gc.clustersEvicted > 0 && e.telemetry != nil {
		e.telemetry.recordLogPatternCountDelta(e.Name(), -float64(gc.clustersEvicted))
	}
	message := log.GetContent()
	groupTags := tagsForPatternGrouping(log.Tags(), log.GetHostname())
	groupHash, cluster, ok := e.taggedClusterer.Process(groupTags, message, logUnixSec)
	// Drain LRU evictions — from per-group MaxClusters or whole-group MaxTagGroups
	// caps. Treated identically to GC evictions: drop engine context, decrement
	// pattern_count telemetry. Done unconditionally so that whole-group evictions
	// caused by the new sub-clusterer creation aren't lost when Process returns
	// !ok (defensive; current Process only returns !ok for empty messages, after
	// which no eviction can have occurred, but keep this path honest).
	if evicted := e.taggedClusterer.DrainLRUEvictions(); len(evicted) > 0 {
		for _, ev := range evicted {
			name := "log." + e.Name() + "." + globalClusterHash(ev.GroupHash, ev.ClusterID) + ".count"
			result.EvictedMetricNames = append(result.EvictedMetricNames, name)
		}
		if e.telemetry != nil {
			e.telemetry.recordLogPatternCountDelta(e.Name(), -float64(len(evicted)))
		}
	}
	if !ok {
		return result
	}
	// Not enough patterns yet, don't emit metric
	// It's not directly a new pattern but the first time we reach the threshold and we emit a metric
	if cluster.Count == e.config.MinClusterSizeBeforeEmit && e.telemetry != nil {
		e.telemetry.recordLogPatternCountDelta(e.Name(), 1)
	} else if cluster.Count < e.config.MinClusterSizeBeforeEmit {
		return result
	}

	metricName := "log." + e.Name() + "." + globalClusterHash(groupHash, cluster.ID) + ".count"

	group, _ := e.registry.Lookup(groupHash)
	result.Metrics = []observerdef.MetricOutput{{
		Name:  metricName,
		Value: 1,
		Tags:  log.Tags(),
		Context: &observerdef.MetricContext{
			Pattern:   cluster.PatternString(),
			Example:   truncate(message, 160),
			Source:    e.Name(),
			SplitTags: group.AsMap(),
		},
	}}
	return result
}

// gcResult holds what was evicted during a garbage-collection pass.
type gcResult struct {
	metricNames     []string // metric names whose series should be removed from storage
	clustersEvicted int
}

// maybeGarbageCollect removes stale clusters from all sub-clusterers and
// returns the context keys evicted so the engine can drop matching contextRefs.
func (e *LogPatternExtractor) maybeGarbageCollect(currentTime int64) gcResult {
	if e.config.ClusterTimeToLiveSec == 0 || currentTime < e.NextGarbageCollectionTime {
		return gcResult{}
	}
	e.NextGarbageCollectionTime = currentTime + e.config.GarbageCollectionIntervalSec

	cutoff := currentTime - e.config.ClusterTimeToLiveSec
	evicted := e.taggedClusterer.GarbageCollectBefore(cutoff)
	if len(evicted) == 0 {
		return gcResult{}
	}
	var result gcResult
	for _, ev := range evicted {
		name := "log." + e.Name() + "." + globalClusterHash(ev.GroupHash, ev.ClusterID) + ".count"
		result.metricNames = append(result.metricNames, name)
	}
	result.clustersEvicted = len(evicted)
	return result
}
