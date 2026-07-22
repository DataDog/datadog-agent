// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strconv"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

// LogTokenizerFuzzyExtractorName is the canonical name for Logs-tokenizer
// patterns clustered with the adaptive sampler's positional match function.
const LogTokenizerFuzzyExtractorName = "log_tokenizer_fuzzy_extractor"

const defaultLogTokenizerMatchThreshold = 0.9

// LogTokenizerFuzzyExtractorConfig controls fuzzy structural-pattern extraction.
type LogTokenizerFuzzyExtractorConfig struct {
	MaxEvalBytes              int     `json:"max_eval_bytes,omitempty"`
	MinPatternCountBeforeEmit int     `json:"min_pattern_count_before_emit,omitempty"`
	MatchThreshold            float64 `json:"match_threshold,omitempty"`
}

// DefaultLogTokenizerFuzzyExtractorConfig returns settings aligned with the
// adaptive sampler and the semantic extractor's emit threshold.
func DefaultLogTokenizerFuzzyExtractorConfig() LogTokenizerFuzzyExtractorConfig {
	return LogTokenizerFuzzyExtractorConfig{
		MaxEvalBytes:              defaultLogTokenizerMaxEvalBytes,
		MinPatternCountBeforeEmit: defaultLogTokenizerMinPatternCountBeforeEmit,
		MatchThreshold:            defaultLogTokenizerMatchThreshold,
	}
}

type fuzzyLogPattern struct {
	tokens []logpattern.Token
	count  int
}

// LogTokenizerFuzzyExtractor mirrors adaptive sampling's first-match scan and
// frequency ordering, but emits count metrics instead of sampling logs.
// Patterns are scoped independently by source/service/env/host.
type LogTokenizerFuzzyExtractor struct {
	config    LogTokenizerFuzzyExtractorConfig
	tokenizer *logpattern.Tokenizer
	groups    map[uint64][]fuzzyLogPattern
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

	return &LogTokenizerFuzzyExtractor{
		config:    cfg,
		tokenizer: logpattern.NewTokenizer(cfg.MaxEvalBytes),
		groups:    make(map[uint64][]fuzzyLogPattern),
	}
}

// Name returns the extractor name.
func (e *LogTokenizerFuzzyExtractor) Name() string {
	return LogTokenizerFuzzyExtractorName
}

// Reset clears fuzzy-pattern state for a fresh replay.
func (e *LogTokenizerFuzzyExtractor) Reset() {
	clear(e.groups)
}

// ProcessLog assigns a log to the first matching representative, scanning the
// most frequently observed representatives first, like adaptive sampling.
func (e *LogTokenizerFuzzyExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	content := log.GetContent()
	tokens, _ := e.tokenizer.Tokenize([]byte(content))
	if len(tokens) == 0 {
		return observerdef.LogMetricsExtractorOutput{}
	}

	group := extractTagGroupByKey(tagsForPatternGrouping(log.Tags(), log.GetHostname()))
	groupHash := tagGroupByKeyHash(group)
	patterns := e.groups[groupHash]
	matched := -1
	for i := range patterns {
		if logpattern.IsMatch(patterns[i].tokens, tokens, e.config.MatchThreshold) {
			matched = i
			break
		}
	}

	if matched < 0 {
		patterns = append(patterns, fuzzyLogPattern{tokens: tokens, count: 1})
		matched = len(patterns) - 1
	} else {
		patterns[matched].count++
		for matched > 0 && patterns[matched-1].count < patterns[matched].count {
			patterns[matched-1], patterns[matched] = patterns[matched], patterns[matched-1]
			matched--
		}
	}
	e.groups[groupHash] = patterns

	pattern := &patterns[matched]
	if pattern.count < e.config.MinPatternCountBeforeEmit {
		return observerdef.LogMetricsExtractorOutput{}
	}

	patternHash := logpattern.Hash(pattern.tokens)
	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{
			Name:  "log." + e.Name() + "." + strconv.FormatUint(patternHash, 16) + ".count",
			Value: 1,
			Tags:  log.Tags(),
			Context: &observerdef.MetricContext{
				Pattern:   logpattern.TokensToString(pattern.tokens),
				Example:   truncate(content, 160),
				Source:    e.Name(),
				SplitTags: group.AsMap(),
			},
		}},
	}
}
