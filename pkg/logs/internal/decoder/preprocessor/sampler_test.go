// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
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
	}, "test")
}

func newSamplerWithProtect(maxPatterns int, burstSize, rateLimit float64, protect bool) *AdaptiveSampler {
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:          maxPatterns,
		RateLimit:            rateLimit,
		BurstSize:            burstSize,
		MatchThreshold:       0.9,
		ProtectImportantLogs: protect,
	}, "test")
}

func testMsg() *message.Message {
	return message.NewMessage([]byte("test"), nil, message.StatusInfo, 0)
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

func tokenize(s string) []Token {
	tok := NewTokenizer(0)
	tokens, _ := tok.Tokenize([]byte(s))
	return tokens
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
	assert.Same(t, msg, s.Process(msg, nil))
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

// --- AdaptiveSampler: misc ---

func TestAdaptiveSampler_FlushReturnsNil(t *testing.T) {
	s := newSampler(10, 10.0, 1.0)
	s.Process(testMsg(), patternA)
	assert.Nil(t, s.Flush())
}

// A message with no tokens (e.g. empty log line) never matches an existing entry
// and is always treated as a new pattern.
func TestAdaptiveSampler_EmptyTokensNewPattern(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	msg := testMsg()
	out := s.Process(msg, nil)
	assert.NotNil(t, out, "empty-token message should be allowed as new pattern")
	require.Len(t, s.entries, 1)
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

// isImportant returns false for tokens that contain no critical keywords.
func TestIsImportant(t *testing.T) {
	assert.True(t, isImportant(tokenize("FATAL: disk full")))
	assert.True(t, isImportant(tokenize("[ERROR] request failed")))
	assert.True(t, isImportant(tokenize("WARNING: low memory")))
	assert.True(t, isImportant(tokenize("PANIC in goroutine")))
	assert.True(t, isImportant(tokenize("CRITICAL: service down")))
	assert.True(t, isImportant(tokenize("EXCEPTION in handler")))
	assert.True(t, isImportant(tokenize("DEADLOCK detected")))
	assert.True(t, isImportant(tokenize("TIMEOUT connecting")))
	assert.True(t, isImportant(tokenize("CRASH dump generated")))
	assert.True(t, isImportant(tokenize("request FAILED")))

	assert.False(t, isImportant(tokenize("info: all good")))
	assert.False(t, isImportant(tokenize("debug: cache hit")))
	assert.False(t, isImportant(tokenize("request processed successfully")))
	assert.False(t, isImportant(nil))
	assert.False(t, isImportant([]Token{}))
}
