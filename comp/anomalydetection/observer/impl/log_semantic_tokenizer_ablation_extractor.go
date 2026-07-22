// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"strconv"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/patterns"
)

// LogSemanticTokenizerAblationExtractorName is an experiment-only extractor
// used to isolate semantic tokenization from the semantic clusterer's matching
// rules. It applies the adaptive sampler's fixed-representative matching
// algorithm to the semantic tokenizer's (type, raw value) tokens.
const LogSemanticTokenizerAblationExtractorName = "log_semantic_tokenizer_ablation_extractor"

const (
	semanticAblationMatchModeAdaptive    = "adaptive"
	semanticAblationMatchModeEqualLength = "equal_length"
	semanticAblationMatchModeExact       = "exact"
)

// LogSemanticTokenizerAblationExtractorConfig controls the semantic-tokenizer
// ablation. MatchMode accepts adaptive, equal_length, or exact.
type LogSemanticTokenizerAblationExtractorConfig struct {
	MaxEvalBytes              int     `json:"max_eval_bytes,omitempty"`
	MinPatternCountBeforeEmit int     `json:"min_pattern_count_before_emit,omitempty"`
	MatchThreshold            float64 `json:"match_threshold,omitempty"`
	MatchMode                 string  `json:"match_mode,omitempty"`
}

// DefaultLogSemanticTokenizerAblationExtractorConfig returns the controlled
// adaptive-match settings used by the experiment.
func DefaultLogSemanticTokenizerAblationExtractorConfig() LogSemanticTokenizerAblationExtractorConfig {
	return LogSemanticTokenizerAblationExtractorConfig{
		MaxEvalBytes:              defaultLogTokenizerMaxEvalBytes,
		MinPatternCountBeforeEmit: defaultLogTokenizerMinPatternCountBeforeEmit,
		MatchThreshold:            0.5,
		MatchMode:                 semanticAblationMatchModeAdaptive,
	}
}

type semanticValueKey struct {
	typ   patterns.TokenType
	value string
}

type semanticAblationPattern struct {
	keys    []semanticValueKey
	display string
	count   int
}

// LogSemanticTokenizerAblationExtractor keeps the rich tokenizer but replaces
// the type-aware PatternClusterer with a simple fixed-representative matcher.
// This is intentionally an experimental control, not a production candidate.
type LogSemanticTokenizerAblationExtractor struct {
	config       LogSemanticTokenizerAblationExtractorConfig
	tokenizer    *patterns.Tokenizer
	groups       map[uint64][]semanticAblationPattern
	exactIndices map[uint64]map[uint64][]int
}

var _ observerdef.LogMetricsExtractor = (*LogSemanticTokenizerAblationExtractor)(nil)

// NewLogSemanticTokenizerAblationExtractor creates the experiment-only
// semantic-tokenizer ablation extractor.
func NewLogSemanticTokenizerAblationExtractor(cfg LogSemanticTokenizerAblationExtractorConfig) *LogSemanticTokenizerAblationExtractor {
	defaults := DefaultLogSemanticTokenizerAblationExtractorConfig()
	if cfg.MaxEvalBytes <= 0 {
		cfg.MaxEvalBytes = defaults.MaxEvalBytes
	}
	if cfg.MinPatternCountBeforeEmit <= 0 {
		cfg.MinPatternCountBeforeEmit = defaults.MinPatternCountBeforeEmit
	}
	if cfg.MatchThreshold <= 0 || cfg.MatchThreshold > 1 {
		cfg.MatchThreshold = defaults.MatchThreshold
	}
	switch cfg.MatchMode {
	case semanticAblationMatchModeAdaptive, semanticAblationMatchModeEqualLength, semanticAblationMatchModeExact:
	default:
		cfg.MatchMode = defaults.MatchMode
	}

	tokenizer := patterns.NewTokenizer()
	tokenizer.MaxStringLen = cfg.MaxEvalBytes

	return &LogSemanticTokenizerAblationExtractor{
		config:       cfg,
		tokenizer:    tokenizer,
		groups:       make(map[uint64][]semanticAblationPattern),
		exactIndices: make(map[uint64]map[uint64][]int),
	}
}

// Name returns the extractor's storage namespace.
func (e *LogSemanticTokenizerAblationExtractor) Name() string {
	return LogSemanticTokenizerAblationExtractorName
}

// Reset clears all pattern state for a fresh replay.
func (e *LogSemanticTokenizerAblationExtractor) Reset() {
	clear(e.groups)
	clear(e.exactIndices)
}

func semanticValueKeys(tokens []patterns.Token) []semanticValueKey {
	keys := make([]semanticValueKey, len(tokens))
	for i, token := range tokens {
		keys[i] = semanticValueKey{typ: token.Type, value: token.Value}
	}
	return keys
}

func semanticAblationIsMatch(a, b []semanticValueKey, mode string, threshold float64) bool {
	if mode == semanticAblationMatchModeExact {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	if mode == semanticAblationMatchModeEqualLength && len(a) != len(b) {
		return false
	}
	count := min(len(a), len(b))
	if count == 0 {
		return len(a) == len(b)
	}
	requiredMatches := int(math.Round(threshold * float64(count)))
	matches := 0
	for i := 0; i < count; i++ {
		if a[i] == b[i] {
			matches++
		}
		if matches+(count-i-1) < requiredMatches {
			return false
		}
	}
	return true
}

func semanticValueKeysHash(keys []semanticValueKey) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, key := range keys {
		binary.LittleEndian.PutUint64(buf[:], uint64(key.typ))
		_, _ = h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], uint64(len(key.value)))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(key.value))
	}
	return h.Sum64()
}

func semanticValueKeysDisplay(keys []semanticValueKey) string {
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key.value)
	}
	return b.String()
}

func (e *LogSemanticTokenizerAblationExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	content := log.GetContent()
	keys := semanticValueKeys(e.tokenizer.Tokenize(content))
	if len(keys) == 0 {
		return observerdef.LogMetricsExtractorOutput{}
	}

	group := extractTagGroupByKey(tagsForPatternGrouping(log.Tags(), log.GetHostname()))
	groupHash := tagGroupByKeyHash(group)
	groupPatterns := e.groups[groupHash]
	matched := -1
	patternHash := semanticValueKeysHash(keys)
	if e.config.MatchMode == semanticAblationMatchModeExact {
		for _, i := range e.exactIndices[groupHash][patternHash] {
			if semanticAblationIsMatch(groupPatterns[i].keys, keys, e.config.MatchMode, e.config.MatchThreshold) {
				matched = i
				break
			}
		}
	} else {
		for i := range groupPatterns {
			if semanticAblationIsMatch(groupPatterns[i].keys, keys, e.config.MatchMode, e.config.MatchThreshold) {
				matched = i
				break
			}
		}
	}

	if matched < 0 {
		groupPatterns = append(groupPatterns, semanticAblationPattern{
			keys:    keys,
			display: semanticValueKeysDisplay(keys),
			count:   1,
		})
		matched = len(groupPatterns) - 1
		if e.config.MatchMode == semanticAblationMatchModeExact {
			if e.exactIndices[groupHash] == nil {
				e.exactIndices[groupHash] = make(map[uint64][]int)
			}
			e.exactIndices[groupHash][patternHash] = append(e.exactIndices[groupHash][patternHash], matched)
		}
	} else {
		groupPatterns[matched].count++
		if e.config.MatchMode != semanticAblationMatchModeExact {
			for matched > 0 && groupPatterns[matched-1].count < groupPatterns[matched].count {
				groupPatterns[matched-1], groupPatterns[matched] = groupPatterns[matched], groupPatterns[matched-1]
				matched--
			}
		}
	}
	e.groups[groupHash] = groupPatterns

	pattern := &groupPatterns[matched]
	if pattern.count < e.config.MinPatternCountBeforeEmit {
		return observerdef.LogMetricsExtractorOutput{}
	}

	patternHash = semanticValueKeysHash(pattern.keys)
	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{
			Name:  "log." + e.Name() + "." + strconv.FormatUint(patternHash, 16) + ".count",
			Value: 1,
			Tags:  log.Tags(),
			Context: &observerdef.MetricContext{
				Pattern:   pattern.display,
				Example:   truncate(content, 160),
				Source:    e.Name(),
				SplitTags: group.AsMap(),
			},
		}},
	}
}
