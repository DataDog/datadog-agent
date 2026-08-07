// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// helpers

func newSampler(maxPatterns int, burstSize, rateLimit float64) *AdaptiveSampler {
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    maxPatterns,
		RateLimit:      rateLimit,
		BurstSize:      burstSize,
		MatchThreshold: 0.9,
	}, "test", 0)
}

func newSamplerWithProtect(maxPatterns int, burstSize, rateLimit float64, protect bool) *AdaptiveSampler {
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:          maxPatterns,
		RateLimit:            rateLimit,
		BurstSize:            burstSize,
		MatchThreshold:       0.9,
		ProtectImportantLogs: protect,
	}, "test", 0)
}

func testMsg() *message.Message {
	return message.NewMessage([]byte("test"), nil, message.StatusInfo, 0)
}

func testMsgWith(content, status string) *message.Message {
	return message.NewMessage([]byte(content), nil, status, 0)
}

func requireSampledCountTag(t *testing.T, msg *message.Message, want int64) {
	t.Helper()
	assert.Contains(t, msg.ParsingExtra.Tags, adaptiveSamplerSampledCountTag(want))
}

func requireNoSampledCountTag(t *testing.T, msg *message.Message) {
	t.Helper()
	for _, tag := range msg.ParsingExtra.Tags {
		assert.NotContains(t, tag, "adaptive_sampler_sampled_count:")
	}
}

func requireTagWithPrefix(t *testing.T, msg *message.Message, prefix string) string {
	t.Helper()
	for _, tag := range msg.ParsingExtra.Tags {
		if strings.HasPrefix(tag, prefix) {
			return tag
		}
	}
	require.Failf(t, "missing tag", "expected tag with prefix %q in %v", prefix, msg.ParsingExtra.Tags)
	return ""
}

func requireNoTagWithPrefix(t *testing.T, msg *message.Message, prefix string) {
	t.Helper()
	for _, tag := range msg.ParsingExtra.Tags {
		assert.Falsef(t, strings.HasPrefix(tag, prefix), "unexpected tag %q with prefix %q", tag, prefix)
	}
}

func tokenize(s string) BorrowedTokens {
	tok := NewTokenizer(0)
	return newBorrowedTokens(tok.Tokenize([]byte(s)))
}

var (
	// structurally similar INFO log lines — match each other at 0.9 threshold
	patternA = tokenize("2024-01-15 10:30:45 INFO [service-a] request processed id=123 duration=42ms")
	patternB = tokenize("metric cpu_usage=45.67 host=web-01 env=prod ts=1234567890")
	patternC = tokenize("http GET /api/v2/users 200 42ms peer=10.0.0.1")
)

// --- NoopSampler ---

func TestNoopSampler_AlwaysPassesThrough(t *testing.T) {
	s := NewNoopSampler()
	msg := testMsg()
	assert.Same(t, msg, s.Process(msg, patternA))
	assert.Same(t, msg, s.Process(msg, BorrowedTokens{}))
}

func TestNoopSampler_FlushReturnsNil(t *testing.T) {
	assert.Nil(t, NewNoopSampler().Flush())
}

// --- AdaptiveSampler: new pattern ---

// A new pattern is always allowed through regardless of credits.
func TestAdaptiveSampler_NewPatternIsAllowed(t *testing.T) {
	s := newSampler(10, 1.0, 0)
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out)
	require.Len(t, s.entries, 1)
	assert.Equal(t, int64(1), s.entries[0].matchCount)
	assert.Equal(t, int64(0), s.entries[0].sampled)
	requireNoSampledCountTag(t, out)
}

func TestAdaptiveSamplerRetainsNewPatternTokens(t *testing.T) {
	s := newSampler(10, 1.0, 0)
	tokens := []Token{1, 2}

	s.Process(testMsg(), newBorrowedTokens(tokens, nil))
	tokens[0] = 9

	require.Len(t, s.entries, 1)
	assert.Equal(t, []Token{1, 2}, s.entries[0].tokens)
}

// A new pattern entry starts with BurstSize-1 credits so that the burst
// allowance accounts for the first message already emitted.
func TestAdaptiveSampler_NewPatternCredits(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	s.Process(testMsg(), patternA)
	assert.Equal(t, 4.0, s.entries[0].credits) // BurstSize-1 = 5-1 = 4
}

// --- AdaptiveSampler: rate limiting ---

// After BurstSize messages the pattern is rate-limited.
func TestAdaptiveSampler_RateLimitsAfterBurst(t *testing.T) {
	const burst = 3.0
	s := newSampler(10, burst, 0) // no credit refill
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	// First message creates the pattern entry and is allowed.
	out1 := s.Process(testMsg(), patternA)
	require.NotNil(t, out1, "msg 1 (new pattern) should be allowed")
	requireNoSampledCountTag(t, out1)
	// Subsequent messages consume credits until the burst is exhausted.
	out2 := s.Process(testMsg(), patternA)
	require.NotNil(t, out2, "msg 2 should be allowed")
	requireNoSampledCountTag(t, out2)
	out3 := s.Process(testMsg(), patternA)
	require.NotNil(t, out3, "msg 3 should be allowed")
	requireNoSampledCountTag(t, out3)
	assert.Nil(t, s.Process(testMsg(), patternA), "msg 4 should be dropped — burst exhausted")
	assert.Equal(t, int64(1), s.entries[0].sampled, "dropped messages should increment the suppressed count")
}

// After being rate-limited, credits refill at RateLimit per second.
func TestAdaptiveSampler_CreditsRefillOverTime(t *testing.T) {
	s := newSampler(10, 3.0, 2.0) // 2 credits/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	msg := testMsg()
	s.Process(msg, patternA) // new pattern; credits = 2
	s.Process(msg, patternA) // credits = 1
	s.Process(msg, patternA) // credits = 0
	assert.Nil(t, s.Process(msg, patternA), "should be rate-limited")

	// Advance time by 0.5s → +1.0 credit (0.5s × 2/s).
	s.now = func() time.Time { return t0.Add(500 * time.Millisecond) }
	out := s.Process(msg, patternA)
	require.NotNil(t, out, "should be allowed after credit refill")
	requireSampledCountTag(t, out, 1)
	assert.Equal(t, int64(0), s.entries[0].sampled, "emitting should reset the suppressed count")
}

func TestAdaptiveSampler_TagsSuppressedMatchesAfterLongDelay(t *testing.T) {
	s := newSampler(10, 1.0, 1.0) // 1 log/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	msg := testMsg()
	out1 := s.Process(msg, patternA)
	require.NotNil(t, out1)
	requireNoSampledCountTag(t, out1)

	assert.Nil(t, s.Process(msg, patternA), "second message should be dropped")
	assert.Equal(t, int64(1), s.entries[0].sampled)

	// Wait longer than one rate-limit period before the next allowed message.
	s.now = func() time.Time { return t0.Add(2 * time.Second) }
	out2 := s.Process(msg, patternA)
	require.NotNil(t, out2)
	requireSampledCountTag(t, out2, 1)
	assert.Equal(t, int64(0), s.entries[0].sampled, "emitting should reset the suppressed count")
}

func TestAdaptiveSampler_DetectionOnlyTagsWouldDrop(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
		DetectionOnly:  true,
	}, "test", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	out1 := s.Process(testMsg(), patternA)
	require.NotNil(t, out1, "first message should still be allowed")
	requireNoTagWithPrefix(t, out1, "noisy_log:")

	out2 := s.Process(testMsg(), patternA)
	require.NotNil(t, out2, "detection-only should keep messages that would be dropped")
	assert.Contains(t, out2.ParsingExtra.Tags, adaptiveSamplerNoisyLogTag)
	requireNoSampledCountTag(t, out2)
	assert.Equal(t, int64(0), s.entries[0].sampled, "detection-only should not count kept messages as suppressed")
}

func TestAdaptiveSampler_DetectionOnlyDoesNotEmitSampledCountAfterRefill(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      1,
		BurstSize:      1,
		MatchThreshold: 0.9,
		DetectionOnly:  true,
	}, "test", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	out2 := s.Process(testMsg(), patternA)
	require.NotNil(t, out2)
	assert.Contains(t, out2.ParsingExtra.Tags, adaptiveSamplerNoisyLogTag)
	requireNoSampledCountTag(t, out2)
	assert.Equal(t, int64(0), s.entries[0].sampled)

	s.now = func() time.Time { return t0.Add(time.Second) }
	out3 := s.Process(testMsg(), patternA)
	require.NotNil(t, out3, "the next credited message should still pass through normally")
	requireNoTagWithPrefix(t, out3, "noisy_log:")
	requireNoSampledCountTag(t, out3)
	assert.Equal(t, int64(0), s.entries[0].sampled)
}

// Credits are capped at BurstSize even if a long time has passed.
func TestAdaptiveSampler_CreditsCappedAtBurstSize(t *testing.T) {
	const burst = 3.0
	s := newSampler(10, burst, 100.0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }
	s.Process(testMsg(), patternA) // create entry; credits = 2

	// Advance by 1 hour — without the cap, credits would be astronomical.
	s.now = func() time.Time { return t0.Add(time.Hour) }
	s.Process(testMsg(), patternA)                 // refill triggers cap
	assert.Equal(t, burst-1, s.entries[0].credits) // credits capped, then decremented
}

// --- AdaptiveSampler: pattern isolation ---

// Different structural patterns are tracked independently — each has its own credit pool.
func TestAdaptiveSampler_PatternsTrackedIndependently(t *testing.T) {
	s := newSampler(10, 1.0, 0) // burst of exactly 1 per pattern
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	msg := testMsg()
	// Pattern A gets its burst.
	assert.NotNil(t, s.Process(msg, patternA), "A msg 1 should be allowed")
	assert.Nil(t, s.Process(msg, patternA), "A msg 2 should be dropped")

	// Pattern B is unaffected by A being exhausted.
	assert.NotNil(t, s.Process(msg, patternB), "B msg 1 should be allowed (independent)")
	assert.Nil(t, s.Process(msg, patternB), "B msg 2 should be dropped")

	require.Len(t, s.entries, 2)
}

// --- AdaptiveSampler: bubbling ---

// After multiple hits, a pattern bubbles toward the front of the sorted list
// so it is found faster on the next scan.
func TestAdaptiveSampler_HotPatternBubblesToFront(t *testing.T) {
	s := newSampler(10, 100.0, 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	msg := testMsg()
	// Insert three distinct patterns.
	s.Process(msg, patternA)
	s.Process(msg, patternB)
	s.Process(msg, patternC)
	// All have matchCount=1; insertion order is A, B, C (all equal, stable).

	// Hit patternC repeatedly so it accumulates the highest matchCount.
	for range 5 {
		s.Process(msg, patternC)
	}

	// patternC should have bubbled to the front.
	assert.Equal(t, int64(6), s.entries[0].matchCount)
	// Remaining entries should be in descending matchCount order.
	for i := 1; i < len(s.entries); i++ {
		assert.LessOrEqual(t, s.entries[i].matchCount, s.entries[i-1].matchCount,
			"entries should be in descending matchCount order")
	}
}

// --- AdaptiveSampler: eviction ---

// When the table is full a new pattern evicts the least-frequently-matched one
// (the last entry in the sorted list).
func TestAdaptiveSampler_EvictsLeastFrequentWhenFull(t *testing.T) {
	s := newSampler(2, 100.0, 0) // table holds at most 2 patterns
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	msg := testMsg()
	s.Process(msg, patternA) // entry 0: matchCount=1
	// Hit A again so it has a higher matchCount than B.
	s.Process(msg, patternA) // matchCount=2; bubbles to front
	s.Process(msg, patternB) // entry 1: matchCount=1; table now full

	// patternC is new — table is full, so least-frequent (B, matchCount=1) is evicted.
	out := s.Process(msg, patternC)
	assert.NotNil(t, out, "new pattern should be allowed")
	require.Len(t, s.entries, 2, "table size should stay at MaxPatterns")

	// A should still be present (it had higher matchCount).
	counts := make([]int64, len(s.entries))
	for i, e := range s.entries {
		counts[i] = e.matchCount
	}
	assert.Contains(t, counts, int64(2), "high-frequency pattern A should be retained")
}

// --- AdaptiveSampler: bubbling aliasing ---

// When a matched entry's incremented matchCount exceeds its predecessor's, the
// entry bubbles forward via value swaps. All entry mutations (including
// sampled_count) must complete before bubbling, otherwise the pointer aliases a
// different entry after the swap.
func TestAdaptiveSampler_BubblingAliasesSampledCount(t *testing.T) {
	s := newSampler(10, 1.0, 1.0) // burst=1, rate=1/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	// Create A and bump its matchCount to 3.
	s.Process(testMsg(), patternA)
	s.now = func() time.Time { return t0.Add(1 * time.Second) }
	s.Process(testMsg(), patternA)
	s.now = func() time.Time { return t0.Add(2 * time.Second) }
	s.Process(testMsg(), patternA)
	// entries: [A(mc=3)]

	// Create B and bump its matchCount to 3 (same as A).
	s.now = func() time.Time { return t0.Add(3 * time.Second) }
	s.Process(testMsg(), patternB)
	s.now = func() time.Time { return t0.Add(4 * time.Second) }
	s.Process(testMsg(), patternB)
	s.now = func() time.Time { return t0.Add(5 * time.Second) }
	s.Process(testMsg(), patternB)
	// entries: [A(mc=3), B(mc=3)]

	// Drop a B at the same timestamp (no credit refill). B.matchCount
	// becomes 4, exceeding A's 3, so B bubbles past A. The pointer
	// aliasing causes the sampled_count increment to land on A.
	assert.Nil(t, s.Process(testMsg(), patternB), "B should be dropped (no credits)")

	// Refill B's credits and emit. The emitted message should carry a tag
	// reporting the 1 dropped message above.
	s.now = func() time.Time { return t0.Add(6 * time.Second) }
	out := s.Process(testMsg(), patternB)
	require.NotNil(t, out, "B should be emitted after credit refill")
	requireSampledCountTag(t, out, 1)
}

func TestAdaptiveSampler_DetectionOnlyHashUsesMatchedPatternAfterBubbling(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      1,
		BurstSize:      1,
		MatchThreshold: 0.9,
		DetectionOnly:  true,
		TagPatternHash: true,
	}, "test", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	// Create A and bump its matchCount to 3.
	s.Process(testMsg(), patternA)
	s.now = func() time.Time { return t0.Add(1 * time.Second) }
	s.Process(testMsg(), patternA)
	s.now = func() time.Time { return t0.Add(2 * time.Second) }
	s.Process(testMsg(), patternA)

	// Create B and bump its matchCount to 3, matching A without bubbling yet.
	s.now = func() time.Time { return t0.Add(3 * time.Second) }
	s.Process(testMsg(), patternB)
	s.now = func() time.Time { return t0.Add(4 * time.Second) }
	s.Process(testMsg(), patternB)
	s.now = func() time.Time { return t0.Add(5 * time.Second) }
	s.Process(testMsg(), patternB)

	// A same-timestamp B match has no refill, would be dropped, and bubbles B
	// past A. Detection-only keeps it, so the hash must still be B's pattern.
	out := s.Process(testMsg(), patternB)
	require.NotNil(t, out, "detection-only should keep the would-drop message")
	assert.Contains(t, out.ParsingExtra.Tags, adaptiveSamplerNoisyLogTag)

	hashTag := requireTagWithPrefix(t, out, "log_hash:")
	assert.Equal(t, adaptiveSamplerLogHashTag(patternB.Borrow()), hashTag)
	assert.NotEqual(t, adaptiveSamplerLogHashTag(patternA.Borrow()), hashTag)
}

// --- AdaptiveSampler: misc ---

func TestAdaptiveSampler_FlushReturnsNil(t *testing.T) {
	s := newSampler(10, 10.0, 1.0)
	s.Process(testMsg(), patternA)
	assert.Nil(t, s.Flush())
}

// A message with no content (e.g. empty log line) is ignored by the sampler: it is
// passed through untouched and does not create or match a pattern entry. The guard
// keys off HasContent(), so structured messages that carry metadata are still sampled.
func TestAdaptiveSampler_EmptyContentIgnored(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	msg := message.NewMessage([]byte{}, nil, message.StatusInfo, 0)
	out := s.Process(msg, BorrowedTokens{})
	assert.Same(t, msg, out, "empty-content message should pass through untouched")
	require.Empty(t, s.entries, "empty-content message must not create a pattern entry")
}

func TestAdaptiveSampler_DoesNotTagPatternHashByDefault(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out)
	requireNoTagWithPrefix(t, out, "log_hash:")
}

func TestAdaptiveSampler_TagPatternHashSkipsUnimpactedLogs(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      2,
		MatchThreshold: 0.9,
		TagPatternHash: true,
		Exclude:        []AdaptiveSamplerFilter{{Regex: regexp.MustCompile(`bypass`)}},
	}, "test", 0)

	out1 := s.Process(testMsgWith("bypass me", message.StatusInfo), patternA)
	require.NotNil(t, out1)
	requireNoTagWithPrefix(t, out1, "log_hash:")

	out2 := s.Process(testMsg(), patternA)
	require.NotNil(t, out2, "new patterns should pass through without hash tagging")
	requireNoTagWithPrefix(t, out2, "log_hash:")

	out3 := s.Process(testMsg(), patternA)
	require.NotNil(t, out3, "under-burst matches should pass through without hash tagging")
	requireNoTagWithPrefix(t, out3, "log_hash:")
}

func TestAdaptiveSampler_TagPatternHashSkipsSampledCountLogs(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      1,
		BurstSize:      1,
		MatchThreshold: 0.9,
		TagPatternHash: true,
	}, "test", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	canonical := newBorrowedTokens([]Token{C1, D1, Fslash, C2, D2, Period, C3, D3, Dash, C4}, nil)
	similar := newBorrowedTokens([]Token{C1, D1, Fslash, C2, D2, Period, C3, D3, Dash, D4}, nil)

	out1 := s.Process(testMsg(), canonical)
	require.NotNil(t, out1)
	requireNoTagWithPrefix(t, out1, "log_hash:")

	require.Nil(t, s.Process(testMsg(), similar), "similar pattern should be suppressed after burst is exhausted")

	s.now = func() time.Time { return t0.Add(time.Second) }
	out2 := s.Process(testMsg(), similar)
	require.NotNil(t, out2)
	requireSampledCountTag(t, out2, 1)
	requireNoTagWithPrefix(t, out2, "log_hash:")
}

// --- AdaptiveSampler: important log protection ---

// An important log is always returned even when credits are exhausted.
func TestAdaptiveSampler_ImportantLogBypassesRateLimit(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, true)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	importantTokens := tokenize("ERROR something went wrong")

	// First message creates a pattern and consumes the burst.
	out1 := s.Process(testMsg(), importantTokens)
	require.NotNil(t, out1, "important log should always pass through")

	// Second message would normally be dropped (burst=1, no refill).
	out2 := s.Process(testMsg(), importantTokens)
	require.NotNil(t, out2, "important log should bypass rate limiting")

	// Third, fourth... all pass through.
	for range 10 {
		assert.NotNil(t, s.Process(testMsg(), importantTokens), "important log should never be dropped")
	}
}

// Important logs bypass the pattern table entirely — no entry created.
func TestAdaptiveSampler_ImportantLogDoesNotCreateEntry(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, true)
	importantTokens := tokenize("FATAL: disk full")

	s.Process(testMsg(), importantTokens)
	assert.Empty(t, s.entries, "important logs should not create pattern table entries")
}

// Non-important logs are still rate-limited normally when protection is on.
func TestAdaptiveSampler_NonImportantLogStillDropped(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, true)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	normalTokens := tokenize("info: request processed")

	out1 := s.Process(testMsg(), normalTokens)
	require.NotNil(t, out1, "first message should be allowed (new pattern)")
	assert.Nil(t, s.Process(testMsg(), normalTokens), "second message should be dropped — burst exhausted")
}

// When ProtectImportantLogs is false, important logs are rate-limited like any other.
func TestAdaptiveSampler_ProtectDisabled(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, false)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	importantTokens := tokenize("ERROR something went wrong")

	out1 := s.Process(testMsg(), importantTokens)
	require.NotNil(t, out1, "first message allowed (new pattern)")
	assert.Nil(t, s.Process(testMsg(), importantTokens), "second message should be dropped — protection disabled")
}

// --- AdaptiveSampler: include/exclude filters ---

func TestAdaptiveSampler_IncludeFiltersSampleMatchingLogs(t *testing.T) {
	tests := []struct {
		name   string
		filter AdaptiveSamplerFilter
		msg    *message.Message
		tokens BorrowedTokens
	}{
		{
			name:   "regex",
			filter: AdaptiveSamplerFilter{Regex: regexp.MustCompile(`foo.*bar`)},
			msg:    testMsgWith("foo hello bar", message.StatusDebug),
			tokens: tokenize("foo hello bar"),
		},
		{
			name:   "sample",
			filter: AdaptiveSamplerFilter{SampleTokens: tokenize("my 123 fun log sample").Borrow()},
			msg:    testMsgWith("my 456 fun log sample", message.StatusDebug),
			tokens: tokenize("my 456 fun log sample"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAdaptiveSampler(AdaptiveSamplerConfig{
				MaxPatterns:    10,
				RateLimit:      0,
				BurstSize:      1,
				MatchThreshold: 0.9,
				Include:        []AdaptiveSamplerFilter{tt.filter},
			}, "test", 0)
			t0 := time.Now()
			s.now = func() time.Time { return t0 }

			require.NotNil(t, s.Process(tt.msg, tt.tokens))
			assert.Nil(t, s.Process(tt.msg, tt.tokens), "matching logs should be sampled after the burst is exhausted")
			require.Len(t, s.entries, 1)
		})
	}
}

func TestAdaptiveSampler_IncludeFiltersBypassNonMatchingLogs(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
		Include:        []AdaptiveSamplerFilter{{Regex: regexp.MustCompile(`error`)}},
	}, "test", 0)
	msg := testMsgWith("ordinary info log", message.StatusInfo)
	tokens := tokenize("ordinary info log")

	require.NotNil(t, s.Process(msg, tokens))
	require.NotNil(t, s.Process(msg, tokens))
	assert.Empty(t, s.entries, "non-included logs should bypass the sampler pattern table")
}

func TestAdaptiveSampler_EmptyConfiguredIncludeBypassesAllLogs(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:       10,
		RateLimit:         0,
		BurstSize:         1,
		MatchThreshold:    0.9,
		IncludeConfigured: true,
	}, "test", 0)
	msg := testMsgWith("ordinary info log", message.StatusInfo)
	tokens := tokenize("ordinary info log")

	require.NotNil(t, s.Process(msg, tokens))
	require.NotNil(t, s.Process(msg, tokens))
	assert.Empty(t, s.entries, "an explicitly empty include list should not sample every log")
}

func TestAdaptiveSampler_ExcludeFiltersBypassMatchingLogs(t *testing.T) {
	tests := []struct {
		name   string
		filter AdaptiveSamplerFilter
		msg    *message.Message
		tokens BorrowedTokens
	}{
		{
			name:   "regex",
			filter: AdaptiveSamplerFilter{Regex: regexp.MustCompile(`foo.*bar`)},
			msg:    testMsgWith("foo hello bar", message.StatusDebug),
			tokens: tokenize("foo hello bar"),
		},
		{
			name:   "sample",
			filter: AdaptiveSamplerFilter{SampleTokens: tokenize("my 123 fun log sample").Borrow()},
			msg:    testMsgWith("my 456 fun log sample", message.StatusDebug),
			tokens: tokenize("my 456 fun log sample"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAdaptiveSampler(AdaptiveSamplerConfig{
				MaxPatterns:    10,
				RateLimit:      0,
				BurstSize:      1,
				MatchThreshold: 0.9,
				Exclude:        []AdaptiveSamplerFilter{tt.filter},
			}, "test", 0)

			require.NotNil(t, s.Process(tt.msg, tt.tokens))
			require.NotNil(t, s.Process(tt.msg, tt.tokens))
			assert.Empty(t, s.entries, "excluded logs should bypass the sampler pattern table")
		})
	}
}

func TestAdaptiveSampler_ExcludeTakesPrecedenceOverInclude(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
		Include:        []AdaptiveSamplerFilter{{Regex: regexp.MustCompile(`foo.*bar`)}},
		Exclude:        []AdaptiveSamplerFilter{{SampleTokens: tokenize("foo hello bar").Borrow()}},
	}, "test", 0)
	msg := testMsgWith("foo hello bar", message.StatusInfo)
	tokens := tokenize("foo hello bar")

	require.NotNil(t, s.Process(msg, tokens))
	require.NotNil(t, s.Process(msg, tokens))
	assert.Empty(t, s.entries, "excluded logs should bypass even when they also match include")
}

// isImportant returns false for tokens that contain no critical keywords.
func TestIsImportant(t *testing.T) {
	assert.True(t, isImportant(tokenize("FATAL: disk full").Borrow()))
	assert.True(t, isImportant(tokenize("[ERROR] request failed").Borrow()))
	assert.True(t, isImportant(tokenize("WARNING: low memory").Borrow()))
	assert.True(t, isImportant(tokenize("PANIC in goroutine").Borrow()))
	assert.True(t, isImportant(tokenize("CRITICAL: service down").Borrow()))
	assert.True(t, isImportant(tokenize("EXCEPTION in handler").Borrow()))
	assert.True(t, isImportant(tokenize("DEADLOCK detected").Borrow()))
	assert.True(t, isImportant(tokenize("TIMEOUT connecting").Borrow()))
	assert.True(t, isImportant(tokenize("CRASH dump generated").Borrow()))
	assert.True(t, isImportant(tokenize("request FAILED").Borrow()))

	assert.False(t, isImportant(tokenize("info: all good").Borrow()))
	assert.False(t, isImportant(tokenize("debug: cache hit").Borrow()))
	assert.False(t, isImportant(tokenize("request processed successfully").Borrow()))
	assert.False(t, isImportant(nil))
	assert.False(t, isImportant([]Token{}))
}

func TestAdaptiveSampler_TagBytesDropped(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
	}, "test_tags", 42)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	tokens := tokenize("hello world 123")

	require.NotNil(t, s.Process(testMsg(), tokens))

	before := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags", "false").Get()

	require.Nil(t, s.Process(testMsg(), tokens))

	after := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags", "false").Get()
	assert.Equal(t, float64(42), after-before)
}

func TestAdaptiveSampler_TagBytesDroppedIncludesParsingExtra(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
	}, "test_tags_extra", 10)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	tokens := tokenize("hello world 123")

	require.NotNil(t, s.Process(testMsg(), tokens))

	before := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags_extra", "false").Get()

	msg := testMsg()
	msg.ParsingExtra.Tags = []string{"truncated:single_line", "multiline:aggregate"}
	require.Nil(t, s.Process(msg, tokens))

	after := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags_extra", "false").Get()
	expected := float64(message.AppendTagMetadataBytes(10, []string{"truncated:single_line", "multiline:aggregate"}))
	assert.Equal(t, expected, after-before)
}

func TestAdaptiveSampler_TagBytesDroppedZeroWhenNoTags(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
	}, "test_tags_zero", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	tokens := tokenize("hello world 123")

	require.NotNil(t, s.Process(testMsg(), tokens))

	before := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags_zero", "false").Get()

	require.Nil(t, s.Process(testMsg(), tokens))

	after := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_tags_zero", "false").Get()
	assert.Equal(t, float64(0), after-before, "no tag bytes tracked when base is 0 and no ParsingExtra tags")
}

func TestAdaptiveSampler_DetectionOnly_TracksBytesWithoutDropping(t *testing.T) {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
		DetectionOnly:  true,
	}, "test_detect", 20)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	tokens := tokenize("hello world 123")

	require.NotNil(t, s.Process(testMsg(), tokens))

	beforeBytes := tlmAdaptiveSamplerBytesDropped.WithValues("test_detect", "true").Get()
	beforeTagBytes := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_detect", "true").Get()

	msg := testMsg()
	result := s.Process(msg, tokens)
	require.NotNil(t, result, "detection-only must not drop messages")
	assert.Contains(t, result.ParsingExtra.Tags, adaptiveSamplerNoisyLogTag)

	afterBytes := tlmAdaptiveSamplerBytesDropped.WithValues("test_detect", "true").Get()
	afterTagBytes := tlmAdaptiveSamplerTagBytesDropped.WithValues("test_detect", "true").Get()

	assert.Greater(t, afterBytes-beforeBytes, float64(0), "bytes_dropped should be tracked in detection-only mode")
	assert.Equal(t, float64(20), afterTagBytes-beforeTagBytes, "tag_bytes_dropped should reflect baseBytesEstimate")
}

func TestAdaptiveSampler_IsSourceDisabled(t *testing.T) {
	t0 := time.Now()

	t.Run("disabled source passes all messages through", func(t *testing.T) {
		s := NewAdaptiveSampler(AdaptiveSamplerConfig{
			MaxPatterns:      10,
			RateLimit:        1.0,
			BurstSize:        1,
			MatchThreshold:   0.3,
			IsSourceDisabled: func() bool { return true },
		}, "test", 0)
		s.now = func() time.Time { return t0 }

		tokens := tokenize("connection timeout to host abc")
		assert.NotNil(t, s.Process(testMsg(), tokens), "first message allowed")
		assert.NotNil(t, s.Process(testMsg(), tokens), "second message also allowed — source is disabled")
		assert.NotNil(t, s.Process(testMsg(), tokens), "third message also allowed — no rate limiting")
	})

	t.Run("enabled source rate-limits normally", func(t *testing.T) {
		s := NewAdaptiveSampler(AdaptiveSamplerConfig{
			MaxPatterns:      10,
			RateLimit:        1.0,
			BurstSize:        1,
			MatchThreshold:   0.3,
			IsSourceDisabled: func() bool { return false },
		}, "test", 0)
		s.now = func() time.Time { return t0 }

		tokens := tokenize("connection timeout to host abc")
		assert.NotNil(t, s.Process(testMsg(), tokens), "first message allowed (new pattern)")
		assert.Nil(t, s.Process(testMsg(), tokens), "second message dropped — burst exhausted")
	})

	t.Run("nil IsSourceDisabled behaves as enabled", func(t *testing.T) {
		s := NewAdaptiveSampler(AdaptiveSamplerConfig{
			MaxPatterns:      10,
			RateLimit:        1.0,
			BurstSize:        1,
			MatchThreshold:   0.3,
			IsSourceDisabled: nil,
		}, "test", 0)
		s.now = func() time.Time { return t0 }

		tokens := tokenize("connection timeout to host abc")
		assert.NotNil(t, s.Process(testMsg(), tokens), "first message allowed")
		assert.Nil(t, s.Process(testMsg(), tokens), "second message dropped — nil means enabled")
	})

	t.Run("dynamic toggle mid-stream", func(t *testing.T) {
		disabled := false
		s := NewAdaptiveSampler(AdaptiveSamplerConfig{
			MaxPatterns:      10,
			RateLimit:        1.0,
			BurstSize:        1,
			MatchThreshold:   0.3,
			IsSourceDisabled: func() bool { return disabled },
		}, "test", 0)
		s.now = func() time.Time { return t0 }

		tokens := tokenize("connection timeout to host abc")
		assert.NotNil(t, s.Process(testMsg(), tokens), "first message allowed (new pattern)")
		assert.Nil(t, s.Process(testMsg(), tokens), "second message dropped — enabled, burst exhausted")

		disabled = true
		assert.NotNil(t, s.Process(testMsg(), tokens), "third message allowed — source now disabled")
		assert.NotNil(t, s.Process(testMsg(), tokens), "fourth message allowed — still disabled")

		disabled = false
		assert.Nil(t, s.Process(testMsg(), tokens), "fifth message dropped — re-enabled, credits still exhausted")
	})
}
