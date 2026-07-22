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

// LogTokenizerExtractorName is the canonical name for exact Logs-tokenizer
// pattern counts produced by the Observer.
const LogTokenizerExtractorName = "log_tokenizer_extractor"

const (
	defaultLogTokenizerMaxEvalBytes              = 2048
	defaultLogTokenizerMinPatternCountBeforeEmit = 5
)

// LogTokenizerExtractorConfig controls exact structural-pattern extraction.
type LogTokenizerExtractorConfig struct {
	// MaxEvalBytes caps the number of bytes tokenized from each log.
	MaxEvalBytes int `json:"max_eval_bytes,omitempty"`
	// MinPatternCountBeforeEmit suppresses a series until this many logs with
	// the same exact token sequence have been seen in the same tag group.
	MinPatternCountBeforeEmit int `json:"min_pattern_count_before_emit,omitempty"`
}

// DefaultLogTokenizerExtractorConfig returns settings aligned with adaptive
// sampling's tokenizer input limit and the semantic extractor's emit threshold.
func DefaultLogTokenizerExtractorConfig() LogTokenizerExtractorConfig {
	return LogTokenizerExtractorConfig{
		MaxEvalBytes:              defaultLogTokenizerMaxEvalBytes,
		MinPatternCountBeforeEmit: defaultLogTokenizerMinPatternCountBeforeEmit,
	}
}

type exactLogPatternKey struct {
	groupHash   uint64
	patternHash uint64
}

// LogTokenizerExtractor uses the Logs tokenizer and its exact FNV-1a token
// hash to emit one count series per structural pattern. It deliberately does
// not extract JSON numeric fields or connection-error signals.
//
// The extractor is not thread-safe; Observer invokes extractors serially.
type LogTokenizerExtractor struct {
	config    LogTokenizerExtractorConfig
	tokenizer *logpattern.Tokenizer
	counts    map[exactLogPatternKey]int
}

var _ observerdef.LogMetricsExtractor = (*LogTokenizerExtractor)(nil)

// NewLogTokenizerExtractor creates an exact structural-pattern extractor.
func NewLogTokenizerExtractor(cfg LogTokenizerExtractorConfig) *LogTokenizerExtractor {
	defaults := DefaultLogTokenizerExtractorConfig()
	if cfg.MaxEvalBytes <= 0 {
		cfg.MaxEvalBytes = defaults.MaxEvalBytes
	}
	if cfg.MinPatternCountBeforeEmit <= 0 {
		cfg.MinPatternCountBeforeEmit = defaults.MinPatternCountBeforeEmit
	}

	return &LogTokenizerExtractor{
		config:    cfg,
		tokenizer: logpattern.NewTokenizer(cfg.MaxEvalBytes),
		counts:    make(map[exactLogPatternKey]int),
	}
}

// Name returns the extractor name.
func (e *LogTokenizerExtractor) Name() string {
	return LogTokenizerExtractorName
}

// Reset clears exact-pattern counts for a fresh replay.
func (e *LogTokenizerExtractor) Reset() {
	clear(e.counts)
}

// ProcessLog emits a count after an exact token sequence reaches the configured
// minimum within its source/service/env/host tag group.
func (e *LogTokenizerExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	content := log.GetContent()
	tokens, _ := e.tokenizer.Tokenize([]byte(content))
	if len(tokens) == 0 {
		return observerdef.LogMetricsExtractorOutput{}
	}

	patternHash := logpattern.Hash(tokens)
	group := extractTagGroupByKey(tagsForPatternGrouping(log.Tags(), log.GetHostname()))
	key := exactLogPatternKey{
		groupHash:   tagGroupByKeyHash(group),
		patternHash: patternHash,
	}
	e.counts[key]++
	if e.counts[key] < e.config.MinPatternCountBeforeEmit {
		return observerdef.LogMetricsExtractorOutput{}
	}

	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{
			Name:  "log." + e.Name() + "." + strconv.FormatUint(patternHash, 16) + ".count",
			Value: 1,
			Tags:  log.Tags(),
			Context: &observerdef.MetricContext{
				Pattern:   logpattern.TokensToString(tokens),
				Example:   truncate(content, 160),
				Source:    e.Name(),
				SplitTags: group.AsMap(),
			},
		}},
	}
}
