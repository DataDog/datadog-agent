// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestVocab creates a minimal vocabulary for testing. Token IDs are
// assigned in order — this mirrors how the Python vocabulary.build_vocabulary
// works (special tokens first, then value tokens, then tag slots, etc.).
func buildTestVocab() *scrappyVocab {
	tokens := []string{
		// 0-9: special tokens
		"[PAD]", "[UNK]", "[BOS]", "[EOS]", "[TICK_START]", "[TICK_END]",
		"[SEP]", "[WILD]", "[PATH]", "[M_LOG_PATTERN]",
		// 10-11: outcome tokens
		"[normal]", "[alert]",
		// 12-21: universal value buckets
		"[V_ZERO]", "[V_TINY]", "[V_LOW]", "[V_NORMAL]", "[V_HIGH]",
		"[V_VERY_HIGH]", "[V_EXTREME]", "[V_MEGA]", "[V_GIGA]", "[V_TERA]",
		// delta tokens (asymmetric — more resolution on upside)
		"[DELTA_NEW]", "[DELTA_GONE]",
		"[DELTA_UP_EXTREME]", "[DELTA_UP_LARGE]", "[DELTA_UP_MED]", "[DELTA_UP_SMALL]",
		"[DELTA_STABLE]",
		"[DELTA_DOWN_SMALL]", "[DELTA_DOWN_MED]", "[DELTA_DOWN_LARGE]",
		// 19-26: percentage buckets
		"[V_PCT_0]", "[V_PCT_1]", "[V_PCT_2]", "[V_PCT_3]",
		"[V_PCT_4]", "[V_PCT_5]", "[V_PCT_6]", "[V_PCT_7]",
		// 27-34: latency buckets
		"[V_LAT_0]", "[V_LAT_1]", "[V_LAT_2]", "[V_LAT_3]",
		"[V_LAT_4]", "[V_LAT_5]", "[V_LAT_6]", "[V_LAT_7]",
		// 35-36: boolean
		"[V_FALSE]", "[V_TRUE]",
		// 37-46: tag slots (subset)
		"[TAG_DEVICE_0]", "[TAG_DEVICE_1]",
		"[TAG_CORE_0]", "[TAG_CORE_1]",
		"[TAG_SERVICE_0]", "[TAG_SERVICE_1]",
		"[TAG_HOST_0]",
		"[TAG_LEVEL_INFO]", "[TAG_LEVEL_WARN]", "[TAG_LEVEL_ERROR]",
		"[TAG_SOURCE_0]",
		// 48-49: signature slots
		"[SIG_0]", "[SIG_1]",
		// metric/log words (from synthtel reference)
		"system", "cpu", "user", "mem", "used", "haproxy", "backend",
		"response", "5xx", "failed", "connect", "database", "connection",
		"refused", "get", "api", "http", "idle", "io", "container",
		"s", "r", "w", "q",
	}
	v := &scrappyVocab{
		tokenToID: make(map[string]int, len(tokens)),
		idToToken: make(map[int]string, len(tokens)),
	}
	for i, tok := range tokens {
		v.tokenToID[tok] = i
		v.idToToken[i] = tok
	}
	return v
}

func TestTokenizeMetricName(t *testing.T) {
	vocab := buildTestVocab()

	tokens := tokenizeMetricName("system.cpu.user", vocab)
	require.Len(t, tokens, 3)
	assert.Equal(t, vocab.encode("system"), tokens[0])
	assert.Equal(t, vocab.encode("cpu"), tokens[1])
	assert.Equal(t, vocab.encode("user"), tokens[2])
}

func TestTokenizeMetricName_UnknownWords(t *testing.T) {
	vocab := buildTestVocab()

	tokens := tokenizeMetricName("kubernetes.pod.restarts", vocab)
	// All words map to [UNK] since they're not in test vocab
	for _, tok := range tokens {
		assert.Equal(t, vocab.encode("[UNK]"), tok)
	}
}

func TestTokenizeMetricName_SingleCharKept(t *testing.T) {
	vocab := buildTestVocab()

	// Single-char fragments carry unit/direction semantics (s, w, r, q)
	// and should be kept. They'll map to UNK if not in vocab, which is
	// fine — but known ones (like direction indicators) get real IDs.
	tokens := tokenizeMetricName("system.io.r.cpu", vocab)
	require.Len(t, tokens, 4) // system, io, r, cpu — all kept
	assert.Equal(t, vocab.encode("system"), tokens[0])
}

func TestTokenizeDrainPattern(t *testing.T) {
	vocab := buildTestVocab()

	tokens := tokenizeDrainPattern("Failed to connect to database: connection refused", vocab)
	// "to" is a stopword (skipped twice), rest should tokenize
	expected := []string{"failed", "connect", "database", "connection", "refused"}
	require.Len(t, tokens, len(expected))
	for i, word := range expected {
		assert.Equal(t, vocab.encode(word), tokens[i])
	}
}

func TestTokenizeDrainPattern_Wildcards(t *testing.T) {
	vocab := buildTestVocab()

	tokens := tokenizeDrainPattern("GET /api/v1/* HTTP/1.1 * *", vocab)
	// "get" → word, "/api/v1/*" → [PATH], "HTTP/1.1" → [PATH], "*" → [WILD], "*" → [WILD]
	assert.Equal(t, vocab.encode("get"), tokens[0])
	assert.Equal(t, vocab.encode("[PATH]"), tokens[1])
	assert.Equal(t, vocab.encode("[PATH]"), tokens[2])
	assert.Equal(t, vocab.encode("[WILD]"), tokens[3])
	assert.Equal(t, vocab.encode("[WILD]"), tokens[4])
}

func TestValueBucket(t *testing.T) {
	tests := []struct {
		val    float64
		name   string
		expect string
	}{
		{0, "system.mem.used", "[V_ZERO]"},
		{0.005, "system.net.bytes_sent", "[V_TINY]"},
		{0.5, "system.net.bytes_sent", "[V_LOW]"},
		{42.0, "system.net.bytes_sent", "[V_NORMAL]"},
		{5000.0, "system.net.bytes_sent", "[V_HIGH]"},
		{500000.0, "system.net.bytes_sent", "[V_VERY_HIGH]"},
		{5000000.0, "system.net.bytes_sent", "[V_MEGA]"},
		{500000000.0, "system.net.bytes_sent", "[V_GIGA]"},
		{50000000000.0, "system.net.bytes_sent", "[V_TERA]"},
		// Nanosecond CPU counter should NOT go through pctBucket
		{440648784.0, "container.cpu.usage", "[V_GIGA]"},
		// Percentage
		{23.4, "system.cpu.user", "[V_PCT_1]"},
		{85.0, "system.cpu.idle", "[V_PCT_4]"},
		// Latency
		{0.5, "http.request.latency", "[V_LAT_0]"},
		{1.5, "http.request.latency", "[V_LAT_1]"},
		{150.0, "http.request.duration", "[V_LAT_6]"},
	}
	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			assert.Equal(t, tt.expect, valueBucket(tt.val, tt.name))
		})
	}
}

func TestTagSlotMap(t *testing.T) {
	m := newTagSlotMap()

	// First core value gets slot 0
	assert.Equal(t, "[TAG_CORE_0]", m.getSlotToken("core", "0"))
	// Same value returns same slot
	assert.Equal(t, "[TAG_CORE_0]", m.getSlotToken("core", "0"))
	// New value gets next slot
	assert.Equal(t, "[TAG_CORE_1]", m.getSlotToken("core", "1"))

	// Level is literal
	assert.Equal(t, "[TAG_LEVEL_ERROR]", m.getSlotToken("level", "error"))
	assert.Equal(t, "[TAG_LEVEL_WARN]", m.getSlotToken("level", "warning"))

	// Unknown key returns empty
	assert.Equal(t, "", m.getSlotToken("env", "prod"))
}

func TestSigSlotMap(t *testing.T) {
	m := newSigSlotMap(3, 2) // threshold=3, budget=2

	sig := "CCC CCCC:DDD:DD.DDD"

	// Below threshold: no slot
	m.observe(sig)
	m.observe(sig)
	assert.Equal(t, "", m.getSlotToken(sig))

	// At threshold: gets slot
	m.observe(sig)
	assert.Equal(t, "[SIG_0]", m.getSlotToken(sig))

	// Second signature
	sig2 := "CCCC CCCC CCC"
	for range 3 {
		m.observe(sig2)
	}
	assert.Equal(t, "[SIG_1]", m.getSlotToken(sig2))

	// Budget exhausted: third signature gets nothing
	sig3 := "DDD.DDD.DDD.DDD"
	for range 3 {
		m.observe(sig3)
	}
	assert.Equal(t, "", m.getSlotToken(sig3))
}

func TestTokenizeTick(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)

	series := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.cpu.user", tags: []string{"core:0"}, value: 23.4},
		{namespace: "system-checks-hf", name: "system.mem.used", value: 42000.0},
		{namespace: "system-checks-hf", name: "system.cpu.user", tags: []string{"core:0"}, value: 23.4}, // duplicate
		{namespace: "system-checks-hf", name: "system.cpu.idle", value: 0},                               // zero, never been non-zero → suppressed
	}

	tokens := tok.tokenizeTick(series)

	// Should start with [TICK_START] and end with [TICK_END]
	assert.Equal(t, vocab.encode("[TICK_START]"), tokens[0])
	assert.Equal(t, vocab.encode("[TICK_END]"), tokens[len(tokens)-1])

	// cpu.user duplicate should be deduped, idle suppressed (never been non-zero)
	// Expect: [TICK_START] system cpu user [TAG_CORE_0] [V_PCT_1] [DELTA_NEW] system mem used [V_HIGH] [DELTA_NEW] [TICK_END]
	// system.mem.used=42000 → [V_HIGH] (not a percentage metric, 10K-100K range)
	// Each series gets a [DELTA_NEW] token (first tick)
	// That's 13 tokens total
	assert.Equal(t, 13, len(tokens), "tokens: %v", decodeTokens(vocab, tokens))
}

func TestTokenizeTick_ZeroNeverNonZero(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)

	series := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.cpu.user", value: 0},
	}

	tokens := tok.tokenizeTick(series)
	// Series never been non-zero → suppressed
	assert.Equal(t, 2, len(tokens), "tokens: %v", decodeTokens(vocab, tokens))
}

func TestTokenizeTick_ZeroAfterNonZero(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)

	// Tick 1: non-zero value
	series1 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.cpu.user", value: 50.0},
	}
	tok.tokenizeTick(series1)

	// Tick 2: drops to zero — should NOT be suppressed, should emit with DELTA_ZERO
	series2 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.cpu.user", value: 0},
	}
	tokens := tok.tokenizeTick(series2)

	// [TICK_START] system cpu user [V_ZERO] [DELTA_ZERO] [TICK_END]
	assert.True(t, len(tokens) > 2, "zero-after-nonzero should NOT be suppressed, got: %v", decodeTokens(vocab, tokens))
	// Should contain DELTA_GONE
	found := false
	for _, id := range tokens {
		if id == vocab.encode("[DELTA_GONE]") {
			found = true
		}
	}
	assert.True(t, found, "should contain [DELTA_GONE], got: %v", decodeTokens(vocab, tokens))
}

func TestTokenizeTick_DeltaTokens(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)

	// Tick 1: baseline
	series1 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.mem.used", value: 1000.0},
	}
	tokens1 := tok.tokenizeTick(series1)
	assert.Contains(t, decodeTokens(vocab, tokens1), "[DELTA_NEW]")

	// Tick 2: 20x spike → DELTA_UP_LARGE (10-100x)
	series2 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.mem.used", value: 20000.0},
	}
	tokens2 := tok.tokenizeTick(series2)
	assert.Contains(t, decodeTokens(vocab, tokens2), "[DELTA_UP_LARGE]")

	// Tick 3: stable (19000/20000 = 0.95)
	series3 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.mem.used", value: 19000.0},
	}
	tokens3 := tok.tokenizeTick(series3)
	assert.Contains(t, decodeTokens(vocab, tokens3), "[DELTA_STABLE]")

	// Tick 4: ~3.8x drop (5000/19000 = 0.26) → DELTA_DOWN_MED (0.1-0.33x)
	series4 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.mem.used", value: 5000.0},
	}
	tokens4 := tok.tokenizeTick(series4)
	assert.Contains(t, decodeTokens(vocab, tokens4), "[DELTA_DOWN_MED]")

	// Tick 5: 500x spike → DELTA_UP_EXTREME (>100x)
	series5 := []scrappySeriesInput{
		{namespace: "system-checks-hf", name: "system.mem.used", value: 2500000.0},
	}
	tokens5 := tok.tokenizeTick(series5)
	assert.Contains(t, decodeTokens(vocab, tokens5), "[DELTA_UP_EXTREME]")
}

func TestDeltaFromRatio(t *testing.T) {
	tests := []struct {
		ratio  float64
		expect string
	}{
		{500, tokDeltaUpExtreme},    // >100x
		{50, tokDeltaUpLarge},       // 10-100x
		{5, tokDeltaUpMed},          // 3-10x
		{2, tokDeltaUpSmall},        // 1.5-3x
		{1.0, tokDeltaStable},       // 0.67-1.5x
		{0.8, tokDeltaStable},       // 0.67-1.5x
		{0.5, tokDeltaDownSmall},    // 0.33-0.67x
		{0.2, tokDeltaDownMed},      // 0.1-0.33x
		{0.05, tokDeltaDownLarge},   // <0.1x
	}
	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			assert.Equal(t, tt.expect, deltaFromRatio(tt.ratio))
		})
	}
}

func TestTokenizeTick_DrainPattern(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)

	series := []scrappySeriesInput{
		{
			namespace: "log_pattern_extractor",
			pattern:   "Failed to connect to database: connection refused",
			tags:      []string{"service:webapp"},
			value:     12.0,
		},
	}

	tokens := tok.tokenizeTick(series)

	// [TICK_START] [M_LOG_PATTERN] failed connect database connection refused [TAG_SERVICE_0] [V_NORMAL] [DELTA_NEW] [TICK_END]
	assert.Equal(t, vocab.encode("[TICK_START]"), tokens[0])
	assert.Equal(t, vocab.encode("[M_LOG_PATTERN]"), tokens[1])
	assert.Equal(t, vocab.encode("[TICK_END]"), tokens[len(tokens)-1])
	// Delta token is second-to-last (before TICK_END)
	assert.Equal(t, vocab.encode("[DELTA_NEW]"), tokens[len(tokens)-2])
}

func TestTokenizeTick_StructuralSignature(t *testing.T) {
	vocab := buildTestVocab()
	tok := newScrappyTokenizer(vocab)
	tok.sigSlots.threshold = 1 // lower for testing

	sig := "CCC CCCC:DDD:DD.DDD"
	tok.sigSlots.observe(sig) // meet threshold

	series := []scrappySeriesInput{
		{namespace: "log_metrics_extractor", pattern: sig, value: 5.0},
	}

	tokens := tok.tokenizeTick(series)
	// [TICK_START] [SIG_0] [V_NORMAL] [DELTA_NEW] [TICK_END]
	assert.Equal(t, 5, len(tokens), "tokens: %v", decodeTokens(vocab, tokens))
	assert.Equal(t, vocab.encode("[SIG_0]"), tokens[1])
}

// decodeTokens is a test helper that converts token IDs to strings.
func decodeTokens(vocab *scrappyVocab, ids []int) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		if tok, ok := vocab.idToToken[id]; ok {
			result[i] = tok
		} else {
			result[i] = "[?]"
		}
	}
	return result
}
