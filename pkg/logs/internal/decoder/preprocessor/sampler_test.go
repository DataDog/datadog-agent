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

// TestNoopSampler_AlwaysPassesThrough anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant ContentBytePassthrough — returned message has
//	                                         the same bytes as input.
//	    @invariant NoMessageFabrication — output is either an input
//	                                       message or null.
//
// NoopSampler is the trivial Sampler fulfiller: it returns the
// input message pointer unchanged on every Process call. assert.Same
// pins pointer identity, which is stronger than byte equality.
func TestNoopSampler_AlwaysPassesThrough(t *testing.T) {
	s := NewNoopSampler()
	msg := testMsg()
	assert.Same(t, msg, s.Process(msg, patternA))
	assert.Same(t, msg, s.Process(msg, nil))
}

// TestNoopSampler_FlushReturnsNil anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant Totality — flush returns either a Message or null.
//
// NoopSampler doesn't buffer; flush returns null on every read.
func TestNoopSampler_FlushReturnsNil(t *testing.T) {
	assert.Nil(t, NewNoopSampler().Flush())
}

// --- AdaptiveSampler: new pattern ---

// TestAdaptiveSampler_NewPatternIsAllowed anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee FirstLogAlwaysEmitted
//	rule RegisterNewPattern (adaptive_sampler.allium)
//	    — fires when no existing pattern matches and table has
//	    capacity; emits unconditionally.
//
// A new pattern is always emitted regardless of credit state. The
// rule's `ensures: PatternEntry.created(...)` is verified via
// s.entries having a single fresh entry with matchCount=1.
func TestAdaptiveSampler_NewPatternIsAllowed(t *testing.T) {
	s := newSampler(10, 1.0, 0)
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out)
	require.Len(t, s.entries, 1)
	assert.Equal(t, int64(1), s.entries[0].matchCount)
	assert.Equal(t, int64(0), s.entries[0].sampled)
	requireNoSampledCountTag(t, out)
}

// TestAdaptiveSampler_NewPatternCredits anchors:
//
//	rule RegisterNewPattern (adaptive_sampler.allium)
//	    ensures: PatternEntry.created(credits: config.burst_size - 1.0, ...)
//
// A new entry's initial credit budget is burst_size - 1: the first
// message consumes one credit implicitly (the same message that
// caused the entry to be created).
func TestAdaptiveSampler_NewPatternCredits(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	s.Process(testMsg(), patternA)
	assert.Equal(t, 4.0, s.entries[0].credits) // BurstSize-1 = 5-1 = 4
}

// --- AdaptiveSampler: rate limiting ---

// TestAdaptiveSampler_RateLimitsAfterBurst anchors:
//
//	rule DropMatchingLog (adaptive_sampler.allium)
//	    — fires when an existing pattern matches and credits are
//	    insufficient.
//	surface AdaptiveSampling
//	    @guarantee SteadyStateRateBound — burst_size + rate_limit*T.
//
// With rate_limit=0 and burst=3, the 4th message of a matching
// pattern is dropped. The dropped count increments on the entry.
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

// TestAdaptiveSampler_CreditsRefillOverTime anchors:
//
//	rule EmitMatchingLog (adaptive_sampler.allium)
//	    let elapsed_seconds = seconds_elapsed(entry.last_seen, now)
//	    let refill = elapsed_seconds * config.rate_limit
//	    let capped_credits = min(entry.credits + refill, burst_size)
//
// After credits are exhausted, advancing the clock by N seconds
// refills N * rate_limit credits (capped at burst_size). The first
// re-emission after exhaustion also carries the adaptive_sampler_
// sampled_count tag annotating the prior dropped message — see
// TagAugmentationOnly on the AdaptiveSampling surface.
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

// TestAdaptiveSampler_TagsSuppressedMatchesAfterLongDelay anchors:
//
//	rule EmitMatchingLog (adaptive_sampler.allium)
//	    let prior_sampled_count = entry.sampled_count
//	    ensures: entry.sampled_count = 0
//	    ensures: LogEmitted(message, sampled_count: prior_sampled_count)
//	rule EmitMatchingLog @guidance — the tag attaches when
//	    prior_sampled_count > 0.
//	surface AdaptiveSampling
//	    @guarantee TagAugmentationOnly
//
// The sampled-count tag carries the count from BEFORE the rule
// reset sampled_count to 0. This requires capturing
// prior_sampled_count as a let-binding before the ensures clauses
// take effect.
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

// TestAdaptiveSampler_DetectionOnlyTagsWouldDrop anchors:
//
//	rule DropMatchingLog (adaptive_sampler.allium)
//	    ensures: if config.detection_only:
//	        LogEmitted(message, noisy_log: true, pattern_hash: ...)
//	    ensures: if not config.detection_only: LogDropped(message)
//	    ensures: if config.detection_only:
//	        entry.sampled_count = prior_sampled_count
//	config.detection_only (adaptive_sampler.allium) — "When true,
//	    logs that would be dropped are emitted with noisy_log:true
//	    instead of being dropped."
//
// In detection-only mode the would-be-drop branch of DropMatchingLog
// flips: the message is emitted with the noisy_log tag, the sampled
// count is NOT incremented (sampled_count stays at prior value), and
// no adaptive_sampler_sampled_count tag is added since the rule never
// actually suppressed prior matches.
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

// TestAdaptiveSampler_DetectionOnlyDoesNotEmitSampledCountAfterRefill anchors:
//
//	rule EmitMatchingLog (adaptive_sampler.allium)
//	    ensures: if config.detection_only:
//	        LogEmitted(message, sampled_count: 0)
//	rule EmitMatchingLog @guidance — "Detection-only mode never emits
//	    adaptive_sampler_sampled_count on allowed logs because it does
//	    not actually drop matching logs."
//
// After a noisy_log:true emission under detection-only, the next
// credit-refilled match goes through EmitMatchingLog. Even though
// the prior emission carried noisy_log, detection-only also
// suppresses the adaptive_sampler_sampled_count tag on the refilled
// emission — the sampled_count never increments under detection
// because the message wasn't actually dropped.
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
// TestAdaptiveSampler_CreditsCappedAtBurstSize anchors:
//
//	invariant CreditsCapped (adaptive_sampler.allium)
//	    for entry in PatternEntries: entry.credits <= config.burst_size
//	rule EmitMatchingLog
//	    let capped_credits = min(entry.credits + refill, burst_size)
//
// The refill computation caps at burst_size — an hour of elapsed
// time at rate_limit=100 would give 360000 credits without the cap;
// the cap bounds it to burst_size, then the emission decrements by 1.
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

// TestAdaptiveSampler_PatternsTrackedIndependently anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee TokenAware — pattern classification uses tokens
//	                             via is_match(...).
//	rule EmitMatchingLog / DropMatchingLog
//	    let matches = filter(PatternEntries, e => is_match(...))
//
// Pattern A's exhausted credit bucket has no effect on pattern B's
// freshly-created bucket. Each pattern entry carries its own credit
// state.
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

// TestAdaptiveSampler_HotPatternBubblesToFront anchors:
//
//	rule EmitMatchingLog @guidance (adaptive_sampler.allium) —
//	    the entry is bubbled toward the front of the sorted table
//	    to maintain descending match_count order via a swap chain.
//
// After patternC accumulates the highest matchCount it sits at
// entries[0]. Remaining entries are in descending matchCount order.
// This is a performance optimisation (hot patterns hit early in
// the scan) but is documented in @guidance because it's
// implementation-visible state ordering.
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

// TestAdaptiveSampler_EvictsLeastFrequentWhenFull anchors:
//
//	rule EvictAndRegisterPattern (adaptive_sampler.allium)
//	    let victim = min_by(PatternEntries, e => e.match_count)
//	    ensures: not exists victim
//	    ensures: PatternEntry.created(...)
//	invariant TableBounded
//	    PatternEntries.count <= config.max_patterns
//
// When max_patterns=2 and the table is full, the least-frequently-
// matched entry is evicted to make room for a new pattern. The
// table size remains at max_patterns; the high-frequency pattern A
// is retained.
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

// TestAdaptiveSampler_BubblingAliasesSampledCount anchors:
//
//	rule EmitMatchingLog @guidance (adaptive_sampler.allium) —
//	    "All entry mutations complete before bubbling to avoid
//	    pointer aliasing."
//
// Regression test for a specific implementation bug: the bubble-
// sort swap chain works on values, not pointers; the local pointer
// `e := &s.entries[i]` aliases a different entry after the first
// swap. All mutations to the matched entry (credits, sampled_count,
// last_seen) MUST complete before the bubbling loop starts.
// Failure mode: the sampled_count increment lands on the wrong
// entry, so the carry-tag count drifts off the dropped count.
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

// TestAdaptiveSampler_DetectionOnlyHashUsesMatchedPatternAfterBubbling anchors:
//
//	rule DropMatchingLog (adaptive_sampler.allium)
//	    ensures: if config.detection_only:
//	        LogEmitted(message, noisy_log: true,
//	                   pattern_hash: entry.signature
//	                                  if config.tag_pattern_hash)
//	rule EmitMatchingLog @guidance — "All entry mutations complete
//	    before bubbling to avoid pointer aliasing."
//	config.tag_pattern_hash (adaptive_sampler.allium) — "When true,
//	    noisy_log:true detection-only logs are tagged with
//	    log_hash:<hash>, where the hash is computed from the
//	    canonical structural token sequence for the matched sampler
//	    pattern."
//
// Regression test that the pattern_hash on a detection-only emission
// is computed from the MATCHED entry's signature, not whichever
// entry the bubble-sort swap chain happens to leave at the matched
// index after the match_count bump. Mirrors the
// BubblingAliasesSampledCount aliasing bug, but for the log_hash
// tag path.
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
	assert.Equal(t, adaptiveSamplerLogHashTag(patternB), hashTag)
	assert.NotEqual(t, adaptiveSamplerLogHashTag(patternA), hashTag)
}

// --- AdaptiveSampler: misc ---

// TestAdaptiveSampler_FlushReturnsNil anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee FlushNoop — flush always returns null;
//	                            AdaptiveSampler does not buffer.
//
// After processing a message (which makes an immediate emit-or-drop
// decision), flush still returns nil. There is no pending state
// to drain.
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
	out := s.Process(msg, nil)
	assert.Same(t, msg, out, "empty-content message should pass through untouched")
	require.Empty(t, s.entries, "empty-content message must not create a pattern entry")
}

// TestAdaptiveSampler_EmptyTokensNewPattern anchors:
//
//	contract PatternMatching (adaptive_sampler.allium)
//	    @invariant EmptySemantics — Two empty sequences match;
//	                                 an empty and a non-empty
//	                                 sequence never match.
//
// An empty-token input never matches a non-empty existing pattern,
// so it falls through to RegisterNewPattern. The created entry
// has an empty signature, which would only match a future empty
// input. (Whether this is desirable is a separate concern;
// EmptySemantics describes the predicate's mathematical
// behaviour.)
func TestAdaptiveSampler_EmptyTokensNewPattern(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	msg := testMsg()
	out := s.Process(msg, nil)
	assert.NotNil(t, out, "empty-token message should be allowed as new pattern")
	require.Len(t, s.entries, 1)
}

// TestAdaptiveSampler_DoesNotTagPatternHashByDefault anchors:
//
//	config.tag_pattern_hash (adaptive_sampler.allium) —
//	    "tag_pattern_hash: Boolean = false"
//
// The pattern-hash tag is opt-in: with the default config (which
// also leaves detection_only off), no emission carries log_hash.
func TestAdaptiveSampler_DoesNotTagPatternHashByDefault(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out)
	requireNoTagWithPrefix(t, out, "log_hash:")
}

// TestAdaptiveSampler_TagPatternHashSkipsUnimpactedLogs anchors:
//
//	config.tag_pattern_hash (adaptive_sampler.allium) — "It does
//	    not tag bypassed, new-pattern, protected, under-burst, or
//	    adaptive_sampler_sampled_count:<n> logs."
//
// log_hash only attaches to noisy_log:true detection-only
// emissions (and to filter-bypassed paths is NOT one of them).
// Verifies that new-pattern emissions, under-burst matched
// emissions and filter-bypassed emissions all flow through
// untagged.
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

// TestAdaptiveSampler_TagPatternHashSkipsSampledCountLogs anchors:
//
//	config.tag_pattern_hash (adaptive_sampler.allium) — "It does
//	    not tag bypassed, new-pattern, protected, under-burst, or
//	    adaptive_sampler_sampled_count:<n> logs."
//
// The credit-refill-after-suppression emission carries an
// adaptive_sampler_sampled_count tag (via EmitMatchingLog's
// prior_sampled_count branch). tag_pattern_hash explicitly
// excludes this emission class — the log_hash tag and the
// sampled_count tag never co-occur on the same message.
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

	canonical := []Token{C1, D1, Fslash, C2, D2, Period, C3, D3, Dash, C4}
	similar := []Token{C1, D1, Fslash, C2, D2, Period, C3, D3, Dash, D4}

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

// TestAdaptiveSampler_ImportantLogBypassesRateLimit anchors:
//
//	rule ImportantLogBypass (adaptive_sampler.allium) —
//	    fires when config.protect_important_logs and
//	    is_important(incoming); emits unconditionally.
//	surface AdaptiveSampling
//	    @guarantee ImportantLogProtection — important logs always
//	                                         emit regardless of
//	                                         pattern table state.
//
// With burst=1 and rate_limit=0, a normal pattern would be
// drop-limited after the first message. An important pattern
// (ERROR token) emits indefinitely.
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

// TestAdaptiveSampler_ImportantLogDoesNotCreateEntry anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee ImportantLogProtection — "The bypass is
//	                                          non-disruptive:
//	                                          pattern table state
//	                                          is entirely
//	                                          unaffected."
//
// An important log under protection does NOT register a new
// pattern entry. The pattern table is exactly as it was before
// the call. (Implication: severity-classifying a log doesn't
// poison the pattern table with shapes the sampler will never use
// for rate limiting.)
func TestAdaptiveSampler_ImportantLogDoesNotCreateEntry(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, true)
	importantTokens := tokenize("FATAL: disk full")

	s.Process(testMsg(), importantTokens)
	assert.Empty(t, s.entries, "important logs should not create pattern table entries")
}

// TestAdaptiveSampler_NonImportantLogStillDropped anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee Totality — the 5 rules partition the input
//	                           space; non-important logs fall to
//	                           the pattern-table rules even when
//	                           protect_important_logs is enabled.
//
// Enabling protection doesn't suppress the standard rate limiting
// for ordinary logs — only the is_important branch is rerouted.
// Pattern-table rules still apply to anything is_important rejects.
func TestAdaptiveSampler_NonImportantLogStillDropped(t *testing.T) {
	s := newSamplerWithProtect(10, 1.0, 0, true)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	normalTokens := tokenize("info: request processed")

	out1 := s.Process(testMsg(), normalTokens)
	require.NotNil(t, out1, "first message should be allowed (new pattern)")
	assert.Nil(t, s.Process(testMsg(), normalTokens), "second message should be dropped — burst exhausted")
}

// TestAdaptiveSampler_ProtectDisabled anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee ImportantLogProtection (negative case) —
//	                "When config.protect_important_logs is false,
//	                 the predicate has no effect — the four
//	                 pattern-table rules apply uniformly regardless
//	                 of severity."
//
// With protection disabled, an ERROR log is rate-limited just like
// any other. The is_important branch of the dispatch is unreachable
// because ImportantLogBypass's first `requires:
// config.protect_important_logs` clause fails.
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
		tokens []Token
	}{
		{
			name:   "regex",
			filter: AdaptiveSamplerFilter{Regex: regexp.MustCompile(`foo.*bar`)},
			msg:    testMsgWith("foo hello bar", message.StatusDebug),
			tokens: tokenize("foo hello bar"),
		},
		{
			name:   "sample",
			filter: AdaptiveSamplerFilter{SampleTokens: tokenize("my 123 fun log sample")},
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
		tokens []Token
	}{
		{
			name:   "regex",
			filter: AdaptiveSamplerFilter{Regex: regexp.MustCompile(`foo.*bar`)},
			msg:    testMsgWith("foo hello bar", message.StatusDebug),
			tokens: tokenize("foo hello bar"),
		},
		{
			name:   "sample",
			filter: AdaptiveSamplerFilter{SampleTokens: tokenize("my 123 fun log sample")},
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
		Exclude:        []AdaptiveSamplerFilter{{SampleTokens: tokenize("foo hello bar")}},
	}, "test", 0)
	msg := testMsgWith("foo hello bar", message.StatusInfo)
	tokens := tokenize("foo hello bar")

	require.NotNil(t, s.Process(msg, tokens))
	require.NotNil(t, s.Process(msg, tokens))
	assert.Empty(t, s.entries, "excluded logs should bypass even when they also match include")
}

// isImportant returns false for tokens that contain no critical keywords.
// TestIsImportant anchors:
//
//	rule ImportantLogBypass @guidance (adaptive_sampler.allium) —
//	    "The implementation checks for critical-severity keyword
//	    tokens (FATAL, ERROR, PANIC, ALERT, SEVERE, CRITICAL,
//	    EMERGENCY, WARN, EXCEPTION, CRASH, FAILURE, DEADLOCK,
//	    TIMEOUT)."
//
// Verifies the is_important predicate's keyword list matches the
// list documented in @guidance. Update both together when adding
// a new severity keyword: the test grows a row; the @guidance
// prose grows a name.
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
