// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"math"
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

// --- AdaptiveSampler: LogEmission.TagPresence ---

// When sampled_count > 0 the emitted message carries the tag.
// When sampled_count = 0 no tag is added.
func TestAdaptiveSampler_TagPresence(t *testing.T) {
	s := newSampler(10, 1.0, 1.0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	// First emit: new pattern, sampled_count = 0 → no tag.
	out1 := s.Process(testMsg(), patternA)
	require.NotNil(t, out1)
	requireNoSampledCountTag(t, out1)

	// Second message dropped (burst exhausted) → sampled_count increments.
	assert.Nil(t, s.Process(testMsg(), patternA))

	// Third emit after refill: sampled_count = 1 → tag present.
	s.now = func() time.Time { return t0.Add(2 * time.Second) }
	out2 := s.Process(testMsg(), patternA)
	require.NotNil(t, out2)
	requireSampledCountTag(t, out2, 1)

	// Fourth emit immediately: sampled_count was reset to 0 → no tag.
	s.now = func() time.Time { return t0.Add(3 * time.Second) }
	out3 := s.Process(testMsg(), patternA)
	require.NotNil(t, out3)
	requireNoSampledCountTag(t, out3)
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

// --- AdaptiveSampler: credit edge cases ---

// Dropping a message does not consume a credit. After a drop, the credit
// balance reflects only the refill, not a decrement.
func TestAdaptiveSampler_DropDoesNotConsumeCredit(t *testing.T) {
	s := newSampler(10, 2.0, 1.0) // burst=2, rate=1/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	s.Process(testMsg(), patternA) // new pattern; credits = 1
	s.Process(testMsg(), patternA) // credits = 0

	// Drop at the same timestamp: no refill, credits stay at 0.
	assert.Nil(t, s.Process(testMsg(), patternA), "should be dropped")
	assert.Equal(t, 0.0, s.entries[0].credits,
		"drop should not decrement credits below 0")

	// Drop again: still 0, not -1.
	assert.Nil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 0.0, s.entries[0].credits,
		"repeated drops should leave credits at 0")
}

// Credits refill even on dropped messages. A drop after a quiet period
// accumulates credits from the elapsed time, enabling the next message.
func TestAdaptiveSampler_RefillHappensOnDrop(t *testing.T) {
	s := newSampler(10, 1.0, 2.0) // burst=1, rate=2/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	s.Process(testMsg(), patternA) // new pattern; credits = 0 (burst=1, first msg costs 1)

	// 0.25s later: refill = 0.25 * 2 = 0.5 credits. Not enough to emit (< 1.0).
	s.now = func() time.Time { return t0.Add(250 * time.Millisecond) }
	assert.Nil(t, s.Process(testMsg(), patternA), "0.5 credits should not be enough")
	// Credits after this drop: 0.5 (refill applied, no decrement).
	assert.Equal(t, 0.5, s.entries[0].credits,
		"drop should preserve refilled credits")

	// 0.25s later again: refill = 0.25 * 2 = 0.5 more → 0.5 + 0.5 = 1.0.
	s.now = func() time.Time { return t0.Add(500 * time.Millisecond) }
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out, "1.0 credits should allow emit")
}

// Credits of exactly 1.0 after refill are sufficient to emit, leaving 0.0.
func TestAdaptiveSampler_ExactlyOneCredit(t *testing.T) {
	s := newSampler(10, 5.0, 2.0) // burst=5, rate=2/sec
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	s.Process(testMsg(), patternA) // new pattern; credits = 4
	s.Process(testMsg(), patternA) // credits = 3
	s.Process(testMsg(), patternA) // credits = 2
	s.Process(testMsg(), patternA) // credits = 1
	s.Process(testMsg(), patternA) // credits = 0
	assert.Nil(t, s.Process(testMsg(), patternA), "should be exhausted")

	// 0.5s * 2/sec = exactly 1.0 credit refill.
	s.now = func() time.Time { return t0.Add(500 * time.Millisecond) }
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out, "exactly 1.0 credits should allow emit")
	assert.Equal(t, 0.0, s.entries[0].credits,
		"emit at exactly 1.0 should leave 0.0 credits")

	// Immediately after: 0.0 credits, should drop.
	assert.Nil(t, s.Process(testMsg(), patternA), "0.0 credits should drop")
}

// --- AdaptiveSampler: CreditRecovery ---
//
// A fully exhausted pattern recovers full burst allowance after a quiet
// period D >= burst_size / rate_limit, and does NOT recover full burst
// when D < burst_size / rate_limit. All cases use burst > rate so that
// recovery takes > 1 second, making the insufficient sub-test non-trivial
// (ceil(burst/rate) >= 2, so ceil-1 >= 1 second of partial recovery).
//
// Integer seconds avoid time.Duration nanosecond truncation. ceil(burst/rate)
// is the smallest whole-second duration guaranteeing full recovery:
// ceil(burst/rate) * rate >= burst by definition, and integer seconds
// survive the time.Duration round-trip exactly.

func TestAdaptiveSampler_CreditRecovery(t *testing.T) {
	cases := []struct {
		name  string
		burst float64
		rate  float64
	}{
		{"burst=3 rate=1", 3, 1},
		{"burst=10 rate=3", 10, 3},
		{"burst=100 rate=7", 100, 7},
		{"burst=50 rate=49", 50, 49},
		{"burst=7 rate=2", 7, 2},
		{"burst=2 rate=1", 2, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			burstInt := int(tc.burst)
			cfg := AdaptiveSamplerConfig{
				MaxPatterns:    10,
				RateLimit:      tc.rate,
				BurstSize:      tc.burst,
				MatchThreshold: 0.9,
			}

			exhaust := func(s *AdaptiveSampler, t0 time.Time) {
				s.now = func() time.Time { return t0 }
				for range burstInt + 5 {
					s.Process(testMsg(), patternA)
				}
				require.Nil(t, s.Process(testMsg(), patternA), "should be rate-limited after exhaustion")
			}

			recoverySec := int64(math.Ceil(tc.burst / tc.rate))

			t.Run("sufficient quiet recovers full burst", func(t *testing.T) {
				s := NewAdaptiveSampler(cfg, "test")
				t0 := time.Now()
				exhaust(s, t0)

				s.now = func() time.Time { return t0.Add(time.Duration(recoverySec) * time.Second) }

				for i := range burstInt {
					require.NotNilf(t, s.Process(testMsg(), patternA),
						"message %d of %d should be emitted after recovery", i+1, burstInt)
				}
				require.Nil(t, s.Process(testMsg(), patternA), "burst should be re-exhausted")
			})

			t.Run("insufficient quiet does not recover full burst", func(t *testing.T) {
				s := NewAdaptiveSampler(cfg, "test")
				t0 := time.Now()
				exhaust(s, t0)

				// One second less than recovery. (recoverySec-1) * rate < burst
				// because all cases have burst > rate, so recoverySec >= 2 and
				// recoverySec-1 >= 1 — always a non-trivial partial recovery.
				s.now = func() time.Time { return t0.Add(time.Duration(recoverySec-1) * time.Second) }

				emitted := 0
				for range burstInt {
					if s.Process(testMsg(), patternA) != nil {
						emitted++
					}
				}
				require.Less(t, emitted, burstInt,
					"insufficient quiet should not restore full burst (got %d/%d)", emitted, burstInt)
				require.Greater(t, emitted, 0,
					"partial recovery should restore some credits")
			})
		})
	}
}

// --- AdaptiveSampler: SteadyStateRateBound ---
//
// Under sustained load at a rate >> rate_limit, the total emitted count
// over T seconds is bounded by burst_size + rate_limit * T. This is the
// core token bucket guarantee.

func TestAdaptiveSampler_SteadyStateRateBound(t *testing.T) {
	cases := []struct {
		name  string
		burst float64
		rate  float64
	}{
		{"burst=5 rate=2", 5, 2},
		{"burst=10 rate=1", 10, 1},
		{"burst=3 rate=3", 3, 3},
		{"burst=20 rate=5", 20, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewAdaptiveSampler(AdaptiveSamplerConfig{
				MaxPatterns:    10,
				RateLimit:      tc.rate,
				BurstSize:      tc.burst,
				MatchThreshold: 0.9,
			}, "test")
			t0 := time.Now()

			// Send messages at 1 per millisecond for 10 seconds.
			// That's 10,000 messages — well above any rate_limit in the cases.
			const totalDuration = 10 * time.Second
			const step = time.Millisecond
			emitted := 0
			for elapsed := time.Duration(0); elapsed <= totalDuration; elapsed += step {
				s.now = func() time.Time { return t0.Add(elapsed) }
				if s.Process(testMsg(), patternA) != nil {
					emitted++
				}
			}

			// Upper bound: burst_size + rate_limit * T.
			T := totalDuration.Seconds()
			upperBound := int(tc.burst + tc.rate*T)
			require.LessOrEqual(t, emitted, upperBound,
				"emitted %d should be <= burst(%v) + rate(%v) * %vs = %d",
				emitted, tc.burst, tc.rate, T, upperBound)

			// Sanity: should have emitted at least rate_limit * T messages
			// (the burst is consumed early, then steady-state takes over).
			lowerBound := int(tc.rate * T)
			require.GreaterOrEqual(t, emitted, lowerBound,
				"emitted %d should be >= rate(%v) * %vs = %d",
				emitted, tc.rate, T, lowerBound)
		})
	}
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

// --- AdaptiveSampler: tie-breaking determinism ---
//
// When multiple entries match an incoming log and share the highest
// matchCount, the entry nearest the front of the table is selected.
// The bubble step uses strict less-than, so equal-count entries never
// swap and their relative order is stable.
//
// Tie-breaking is a structural property of scan order, determined by
// table state (entry positions and matchCounts), not token content.
// Fuzzing token inputs would exercise the tokenizer and IsMatch, not
// the ordering logic, so these are table-driven tests over table states.
//
// The single-position-mutation trick for creating patterns that both
// match an incoming log but don't cross-match only works for sequences
// of length 10-15 at threshold 0.9. At length n: required = round(0.9*n),
// 1-diff gives n-1 matches (pass), 2-diff gives n-2 matches (must fail).
// At n >= 16, n-2 >= required and the patterns cross-match.
//
// These tests use a 10-token base with single-position mutations:
// required = 9, pattern-vs-base = 9/10 (match), pattern-vs-pattern =
// 8/10 (no match). Each pattern matches base independently, so we can
// bump each entry's matchCount in isolation.
func TestAdaptiveSampler_TieBreakingDeterminism(t *testing.T) {
	base := []Token{D4, Dash, D2, Dash, D2, Space, D2, Colon, D2, Colon}
	mutate := func(pos int) []Token {
		out := make([]Token, len(base))
		copy(out, base)
		out[pos] = (base[pos] + 1) % End
		return out
	}
	patX := mutate(3) // differs from base at position 3
	patY := mutate(7) // differs from base at position 7
	patZ := mutate(1) // differs from base at position 1

	// Verify geometry: each pattern matches base, no pair cross-matches.
	require.True(t, IsMatch(patX, base, 0.9))
	require.True(t, IsMatch(patY, base, 0.9))
	require.True(t, IsMatch(patZ, base, 0.9))
	require.False(t, IsMatch(patX, patY, 0.9))
	require.False(t, IsMatch(patX, patZ, 0.9))
	require.False(t, IsMatch(patY, patZ, 0.9))

	// Sending patX/patY/patZ to the sampler matches only the corresponding
	// entry (no cross-match), so we can bump each entry's matchCount
	// independently. Sending base matches all entries that are present.

	t.Run("fresh tie at matchCount=1", func(t *testing.T) {
		// Both entries start at matchCount=1 from insertion.
		// X is at index 0, Y at index 1.
		s := newSampler(10, 100.0, 0)
		s.Process(testMsg(), patX)
		s.Process(testMsg(), patY)

		s.Process(testMsg(), base)
		assert.Equal(t, int64(2), s.entries[0].matchCount,
			"X (first in table order) should be selected")
		assert.Equal(t, int64(1), s.entries[1].matchCount,
			"Y should not be selected")
	})

	t.Run("evolved tie: both climb to matchCount=4 independently", func(t *testing.T) {
		// X and Y are bumped to the same matchCount via independent hits.
		// Because Y's matchCount never exceeds X's during the climb (we
		// bump X first), Y never bubbles past X. Table order is preserved.
		s := newSampler(10, 100.0, 0)
		s.Process(testMsg(), patX) // X: mc=1
		s.Process(testMsg(), patY) // Y: mc=1

		for range 3 {
			s.Process(testMsg(), patX) // X only
		}
		// Table: [X(4), Y(1)]
		for range 3 {
			s.Process(testMsg(), patY) // Y only; never exceeds X, no bubble
		}
		// Table: [X(4), Y(4)]
		require.Equal(t, int64(4), s.entries[0].matchCount)
		require.Equal(t, int64(4), s.entries[1].matchCount)

		s.Process(testMsg(), base)
		assert.Equal(t, int64(5), s.entries[0].matchCount,
			"X (preserved at front through independent evolution) should be selected")
		assert.Equal(t, int64(4), s.entries[1].matchCount,
			"Y should not be selected")
	})

	t.Run("reversed order: Y overtakes X via bubbling, then X catches up", func(t *testing.T) {
		// Y accumulates a higher matchCount than X, bubbles to the front.
		// X later catches up to the same count but does NOT bubble past Y
		// (strict-less-than comparison). Now Y is at front despite X being
		// inserted first.
		s := newSampler(10, 100.0, 0)
		s.Process(testMsg(), patX) // X: mc=1
		s.Process(testMsg(), patY) // Y: mc=1
		// Table: [X(1), Y(1)]

		// Bump Y to mc=3. First hit: Y(2) > X(1) → Y bubbles to front.
		for range 2 {
			s.Process(testMsg(), patY)
		}
		// Table: [Y(3), X(1)]

		// Bump X to mc=3. X never exceeds Y's count, no bubble.
		for range 2 {
			s.Process(testMsg(), patX)
		}
		// Table: [Y(3), X(3)]
		require.Equal(t, int64(3), s.entries[0].matchCount)
		require.Equal(t, int64(3), s.entries[1].matchCount)

		s.Process(testMsg(), base)
		assert.Equal(t, int64(4), s.entries[0].matchCount,
			"Y (bubbled to front when it overtook X) should be selected")
		assert.Equal(t, int64(3), s.entries[1].matchCount,
			"X (now behind Y) should not be selected")
	})

	t.Run("three-way tie", func(t *testing.T) {
		// Three entries all at matchCount=1. First in table order wins.
		s := newSampler(10, 100.0, 0)
		s.Process(testMsg(), patX)
		s.Process(testMsg(), patY)
		s.Process(testMsg(), patZ)
		require.Len(t, s.entries, 3)

		s.Process(testMsg(), base)
		assert.Equal(t, int64(2), s.entries[0].matchCount,
			"X (first of three tied entries) should be selected")
		assert.Equal(t, int64(1), s.entries[1].matchCount,
			"Y should not be selected")
		assert.Equal(t, int64(1), s.entries[2].matchCount,
			"Z should not be selected")
	})
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
