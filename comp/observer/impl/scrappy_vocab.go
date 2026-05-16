// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
)

// Scrappy vocabulary layout: a frozen flat word list, same across all episodes.
//
// The vocabulary is built once from the synthtel reference catalogue (1,781
// metric name words appearing in 5+ metrics) plus ~100-200 log-specific words,
// structural tokens, value buckets, tag slots, and signature slots. Total
// ~2,050-2,250 tokens.
//
// Key design property: the same word always maps to the same ID everywhere.
// Training and inference share the same frozen vocab. Words not in the
// vocabulary produce [UNK] — surrounding words still carry meaning.
//
// See: ~/dd/scrappy/docs/tokenizer-v1.md for the full design rationale.
// See: ~/dd/scrappy/DECISIONS.md P0 for vocab consistency across episodes.
//
// Layout (IDs assigned in order during vocabulary construction):
//
//	[0..11]    special + outcome tokens
//	[12..41]   value bucket tokens
//	[42..116]  tag slot tokens
//	[117..266] signature slot tokens
//	[267..)    metric name words + log-specific words

// scrappyVocab is a flat word vocabulary with bidirectional token↔ID mapping.
// Loaded from a vocab.json file produced by the Python scrappy.vocabulary module.
//
// The vocab.json is built once from the synthtel reference catalogue and frozen.
// All episodes and all production deployments use the same file. Unknown words
// map to [UNK] (token ID 1), which is acceptable — the surrounding words and
// value/tag tokens still carry signal. See tokenizer-v1.md "Vocabulary
// Construction" for word frequency analysis showing 95.3% coverage with the
// top 1,781 metric words.
type scrappyVocab struct {
	tokenToID map[string]int
	idToToken map[int]string
}

// scrappyVocabJSON is the JSON structure of a vocab.json file.
type scrappyVocabJSON struct {
	Tokens []string `json:"tokens"`
}

// loadScrappyVocab loads a vocabulary from a vocab.json file.
func loadScrappyVocab(path string) (*scrappyVocab, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vocab: %w", err)
	}
	var vj scrappyVocabJSON
	if err := json.Unmarshal(data, &vj); err != nil {
		return nil, fmt.Errorf("parse vocab: %w", err)
	}
	v := &scrappyVocab{
		tokenToID: make(map[string]int, len(vj.Tokens)),
		idToToken: make(map[int]string, len(vj.Tokens)),
	}
	for i, tok := range vj.Tokens {
		v.tokenToID[tok] = i
		v.idToToken[i] = tok
	}
	return v, nil
}

// encode returns the token ID for a word, or the [UNK] ID if unknown.
func (v *scrappyVocab) encode(token string) int {
	if id, ok := v.tokenToID[token]; ok {
		return id
	}
	return v.tokenToID["[UNK]"]
}

// contains returns true if the token is in the vocabulary.
func (v *scrappyVocab) contains(token string) bool {
	_, ok := v.tokenToID[token]
	return ok
}

// size returns the vocabulary size.
func (v *scrappyVocab) size() int {
	return len(v.tokenToID)
}

// Well-known token names. IDs are looked up from the loaded vocabulary at
// runtime — they are NOT hardcoded constants because the exact ID depends
// on the vocab.json ordering.
const (
	tokPad        = "[PAD]"
	tokUNK        = "[UNK]"
	tokTickStart  = "[TICK_START]"
	tokTickEnd    = "[TICK_END]"
	tokWild       = "[WILD]"
	tokPath       = "[PATH]"
	tokLogPattern = "[M_LOG_PATTERN]"
	tokNormal     = "[normal]"
	tokAlert      = "[alert]"
	tokVZero      = "[V_ZERO]"
	tokVTiny      = "[V_TINY]"
	tokVLow       = "[V_LOW]"
	tokVNormal    = "[V_NORMAL]"
	tokVHigh      = "[V_HIGH]"
	tokVVeryHigh  = "[V_VERY_HIGH]"
	tokVExtreme   = "[V_EXTREME]"
	tokVMega      = "[V_MEGA]"      // 10^6 to 10^8 (byte counts, large counters)
	tokVGiga      = "[V_GIGA]"      // 10^8 to 10^10 (memory bytes, nanosecond counters)
	tokVTera      = "[V_TERA]"      // > 10^10 (cumulative ns, no-limit sentinels)
	tokVTrue      = "[V_TRUE]"
	tokVFalse     = "[V_FALSE]"

	// Delta tokens — logarithmic rate of change relative to previous tick.
	// Asymmetric: more resolution on the upside since spikes are the primary
	// anomaly signal. The absolute value bucket anchors magnitude; the delta
	// captures motion. They're complementary, not redundant.
	tokDeltaNew       = "[DELTA_NEW]"          // series appeared (was absent last tick)
	tokDeltaGone      = "[DELTA_GONE]"         // series disappeared (went to zero/absent)
	tokDeltaUpExtreme = "[DELTA_UP_EXTREME]"   // >100x increase
	tokDeltaUpLarge   = "[DELTA_UP_LARGE]"     // 10-100x increase
	tokDeltaUpMed     = "[DELTA_UP_MED]"       // 3-10x increase
	tokDeltaUpSmall   = "[DELTA_UP_SMALL]"     // 1.5-3x increase
	tokDeltaStable    = "[DELTA_STABLE]"       // <1.5x change either direction
	tokDeltaDownSmall = "[DELTA_DOWN_SMALL]"   // 1.5-3x decrease
	tokDeltaDownMed   = "[DELTA_DOWN_MED]"     // 3-10x decrease
	tokDeltaDownLarge = "[DELTA_DOWN_LARGE]"   // >10x decrease
)
