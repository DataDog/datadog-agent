// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/binary"
	"hash/fnv"
	"strconv"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

// LogTokenizerFuzzyExtractorName is the canonical name for Logs-tokenizer
// patterns clustered with the Logs pipeline's positional match function.
const LogTokenizerFuzzyExtractorName = "log_tokenizer_fuzzy_extractor"

const (
	defaultLogTokenizerMaxEvalBytes              = 2048
	defaultLogTokenizerMinPatternCountBeforeEmit = 5
	defaultLogTokenizerMatchThreshold            = 0.5
	defaultLogTokenizerMaxPatternsPerGroup       = 1024
	defaultLogTokenizerMaxTagGroups              = 256
	defaultLogTokenizerPatternTTL                = 4 * time.Hour
	defaultLogTokenizerGCInterval                = time.Hour
)

// LogTokenizerFuzzyExtractorConfig controls fuzzy structural-pattern extraction.
type LogTokenizerFuzzyExtractorConfig struct {
	MaxEvalBytes                 int     `json:"max_eval_bytes,omitempty"`
	MinPatternCountBeforeEmit    int     `json:"min_pattern_count_before_emit,omitempty"`
	MatchThreshold               float64 `json:"match_threshold,omitempty"`
	MaxPatternsPerGroup          int     `json:"max_patterns_per_group,omitempty"`
	MaxTagGroups                 int     `json:"max_tag_groups,omitempty"`
	PatternTimeToLiveSec         int64   `json:"pattern_time_to_live_sec,omitempty"`
	GarbageCollectionIntervalSec int64   `json:"garbage_collection_interval_sec,omitempty"`
}

// DefaultLogTokenizerFuzzyExtractorConfig returns the configuration evaluated
// by the Observer experiments. The 0.5 threshold is Observer-specific.
func DefaultLogTokenizerFuzzyExtractorConfig() LogTokenizerFuzzyExtractorConfig {
	return LogTokenizerFuzzyExtractorConfig{
		MaxEvalBytes:                 defaultLogTokenizerMaxEvalBytes,
		MinPatternCountBeforeEmit:    defaultLogTokenizerMinPatternCountBeforeEmit,
		MatchThreshold:               defaultLogTokenizerMatchThreshold,
		MaxPatternsPerGroup:          defaultLogTokenizerMaxPatternsPerGroup,
		MaxTagGroups:                 defaultLogTokenizerMaxTagGroups,
		PatternTimeToLiveSec:         int64(defaultLogTokenizerPatternTTL.Seconds()),
		GarbageCollectionIntervalSec: int64(defaultLogTokenizerGCInterval.Seconds()),
	}
}

type fuzzyLogPattern struct {
	tokens   []logpattern.Token
	count    int
	lastSeen int64
}

type fuzzyLogPatternGroup struct {
	group    TagGroupByKey
	patterns []fuzzyLogPattern
	lastSeen int64
}

// LogTokenizerFuzzyExtractor tokenizes each log with the Logs tokenizer, then
// assigns it to the first representative satisfying positional IsMatch. The
// representatives are ordered by frequency, mirroring adaptive sampling.
// Patterns are scoped independently by source/service/env/host.
type LogTokenizerFuzzyExtractor struct {
	config    LogTokenizerFuzzyExtractorConfig
	tokenizer *logpattern.Tokenizer
	groups    map[uint64]*fuzzyLogPatternGroup
	nextGC    int64
}

var _ observerdef.LogMetricsExtractor = (*LogTokenizerFuzzyExtractor)(nil)

// NewLogTokenizerFuzzyExtractor creates a fuzzy structural-pattern extractor.
func NewLogTokenizerFuzzyExtractor(cfg LogTokenizerFuzzyExtractorConfig) *LogTokenizerFuzzyExtractor {
	defaults := DefaultLogTokenizerFuzzyExtractorConfig()
	if cfg.MaxEvalBytes <= 0 {
		cfg.MaxEvalBytes = defaults.MaxEvalBytes
	}
	if cfg.MinPatternCountBeforeEmit <= 0 {
		cfg.MinPatternCountBeforeEmit = defaults.MinPatternCountBeforeEmit
	}
	if cfg.MatchThreshold <= 0 || cfg.MatchThreshold > 1 {
		cfg.MatchThreshold = defaults.MatchThreshold
	}
	if cfg.MaxPatternsPerGroup == 0 {
		cfg.MaxPatternsPerGroup = defaults.MaxPatternsPerGroup
	}
	if cfg.MaxTagGroups == 0 {
		cfg.MaxTagGroups = defaults.MaxTagGroups
	}
	if cfg.PatternTimeToLiveSec == 0 {
		cfg.PatternTimeToLiveSec = defaults.PatternTimeToLiveSec
	}
	if cfg.GarbageCollectionIntervalSec <= 0 {
		cfg.GarbageCollectionIntervalSec = defaults.GarbageCollectionIntervalSec
	}

	return &LogTokenizerFuzzyExtractor{
		config:    cfg,
		tokenizer: logpattern.NewTokenizer(cfg.MaxEvalBytes),
		groups:    make(map[uint64]*fuzzyLogPatternGroup),
	}
}

// Name returns the extractor name.
func (e *LogTokenizerFuzzyExtractor) Name() string {
	return LogTokenizerFuzzyExtractorName
}

// Reset clears fuzzy-pattern state for a fresh replay.
func (e *LogTokenizerFuzzyExtractor) Reset() {
	clear(e.groups)
	e.nextGC = 0
}

// ProcessLog assigns a log to the first matching representative, scanning the
// most frequently observed representatives first, like adaptive sampling.
func (e *LogTokenizerFuzzyExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	timestamp := log.GetTimestampUnixMilli() / 1000
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	result := observerdef.LogMetricsExtractorOutput{
		EvictedMetricNames: e.garbageCollect(timestamp),
	}

	content := log.GetContent()
	tokens, _ := e.tokenizer.Tokenize([]byte(content))
	if len(tokens) == 0 {
		return result
	}

	group := extractTagGroupByKey(tagsForPatternGrouping(log.Tags(), log.GetHostname()))
	groupHash := tagGroupByKeyHash(group)
	patternGroup := e.groups[groupHash]
	if patternGroup == nil {
		result.EvictedMetricNames = append(result.EvictedMetricNames, e.evictTagGroupIfFull()...)
		patternGroup = &fuzzyLogPatternGroup{group: group}
		e.groups[groupHash] = patternGroup
	}
	patternGroup.lastSeen = timestamp

	patterns := patternGroup.patterns
	matched := -1
	for i := range patterns {
		if logpattern.IsMatch(patterns[i].tokens, tokens, e.config.MatchThreshold) {
			matched = i
			break
		}
	}

	if matched < 0 {
		if e.config.MaxPatternsPerGroup > 0 && len(patterns) >= e.config.MaxPatternsPerGroup {
			var evicted string
			patterns, evicted = e.evictOldestPattern(groupHash, patterns)
			if evicted != "" {
				result.EvictedMetricNames = append(result.EvictedMetricNames, evicted)
			}
		}
		patterns = append(patterns, fuzzyLogPattern{tokens: tokens, count: 1, lastSeen: timestamp})
		matched = len(patterns) - 1
	} else {
		patterns[matched].count++
		patterns[matched].lastSeen = timestamp
		for matched > 0 && patterns[matched-1].count < patterns[matched].count {
			patterns[matched-1], patterns[matched] = patterns[matched], patterns[matched-1]
			matched--
		}
	}
	patternGroup.patterns = patterns

	pattern := &patternGroup.patterns[matched]
	if pattern.count < e.config.MinPatternCountBeforeEmit {
		return result
	}

	result.Metrics = []observerdef.MetricOutput{{
		Name:  e.metricName(groupHash, pattern.tokens),
		Value: 1,
		Tags:  log.Tags(),
		Context: &observerdef.MetricContext{
			Pattern:   logpattern.TokensToString(pattern.tokens),
			Example:   truncate(content, 160),
			Source:    e.Name(),
			SplitTags: patternGroup.group.AsMap(),
		},
	}}
	return result
}

func (e *LogTokenizerFuzzyExtractor) metricName(groupHash uint64, tokens []logpattern.Token) string {
	var input [16]byte
	binary.LittleEndian.PutUint64(input[:8], groupHash)
	binary.LittleEndian.PutUint64(input[8:], logpattern.Hash(tokens))
	h := fnv.New64a()
	_, _ = h.Write(input[:])
	return "log." + e.Name() + "." + strconv.FormatUint(h.Sum64(), 16) + ".count"
}

func (e *LogTokenizerFuzzyExtractor) evictTagGroupIfFull() []string {
	if e.config.MaxTagGroups <= 0 || len(e.groups) < e.config.MaxTagGroups {
		return nil
	}
	var victimHash uint64
	var victim *fuzzyLogPatternGroup
	for hash, group := range e.groups {
		if victim == nil || group.lastSeen < victim.lastSeen ||
			(group.lastSeen == victim.lastSeen && hash < victimHash) {
			victimHash, victim = hash, group
		}
	}
	if victim == nil {
		return nil
	}
	delete(e.groups, victimHash)
	return e.evictedMetricNames(victimHash, victim.patterns)
}

func (e *LogTokenizerFuzzyExtractor) evictOldestPattern(groupHash uint64, patterns []fuzzyLogPattern) ([]fuzzyLogPattern, string) {
	oldest := 0
	for i := 1; i < len(patterns); i++ {
		if patterns[i].lastSeen < patterns[oldest].lastSeen {
			oldest = i
		}
	}
	evicted := ""
	if patterns[oldest].count >= e.config.MinPatternCountBeforeEmit {
		evicted = e.metricName(groupHash, patterns[oldest].tokens)
	}
	copy(patterns[oldest:], patterns[oldest+1:])
	patterns[len(patterns)-1] = fuzzyLogPattern{}
	return patterns[:len(patterns)-1], evicted
}

func (e *LogTokenizerFuzzyExtractor) garbageCollect(timestamp int64) []string {
	if e.config.PatternTimeToLiveSec < 0 || timestamp < e.nextGC {
		return nil
	}
	e.nextGC = timestamp + e.config.GarbageCollectionIntervalSec
	cutoff := timestamp - e.config.PatternTimeToLiveSec
	var evicted []string
	for groupHash, group := range e.groups {
		kept := group.patterns[:0]
		for _, pattern := range group.patterns {
			if pattern.lastSeen < cutoff {
				if pattern.count >= e.config.MinPatternCountBeforeEmit {
					evicted = append(evicted, e.metricName(groupHash, pattern.tokens))
				}
				continue
			}
			kept = append(kept, pattern)
		}
		clear(group.patterns[len(kept):])
		group.patterns = kept
		if len(group.patterns) == 0 {
			delete(e.groups, groupHash)
		}
	}
	return evicted
}

func (e *LogTokenizerFuzzyExtractor) evictedMetricNames(groupHash uint64, patterns []fuzzyLogPattern) []string {
	names := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern.count >= e.config.MinPatternCountBeforeEmit {
			names = append(names, e.metricName(groupHash, pattern.tokens))
		}
	}
	return names
}
