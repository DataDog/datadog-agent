// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis regression guard for adaptive-sampler-no-aliasing. Run:
//
//	go test -tags "antithesis_demo test" -run TestAdaptiveSampler_AliasingRegressionGuard \
//	    ./pkg/logs/internal/decoder/preprocessor/ -v
//
// Exercises the "mutate before bubble" invariant across many interleaving patterns.
// After every Process() call the entry whose tokens matched the input must have its
// matchCount incremented by exactly 1 and its sampled field must be consistent with
// the allow/drop decision. Any aliasing caused by a mutation placed after the bubble
// loop would move the write to a different entry, breaking these invariants.
//
// EXPECTED TO PASS — the fix is present at sampler.go:215-217.
// If this test fails, the aliasing bug has been reintroduced.

package preprocessor

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// TestAdaptiveSampler_AliasingRegressionGuard drives the sampler through many
// sequences of A/B pattern matches that force repeated bubbling. After each
// Process() call it locates the entry that matched (by token identity) and
// verifies that matchCount was incremented on the correct entry, and that the
// sampled field is consistent with the allow/drop decision.
func TestAdaptiveSampler_AliasingRegressionGuard(t *testing.T) {
	const (
		burst     = 2.0
		rateLimit = 1.0 // 1 log/sec so credits refill quickly
		maxPat    = 4   // small table to cause frequent evictions / bubbling
	)

	s := newSampler(maxPat, burst, rateLimit)
	t0 := time.Now()
	step := 0

	advanceTime := func(secs float64) {
		t0 = t0.Add(time.Duration(secs * float64(time.Second)))
		s.now = func() time.Time { return t0 }
	}
	advanceTime(0) // pin time

	// patterns has 3 distinct patterns; we'll drive all of them alternately.
	patterns := [][]Token{patternA, patternB, patternC}
	names := []string{"A", "B", "C"}

	// Drive 200 Process() calls, cycling through all three patterns.
	// Every other step advances time by 0.5s to partially refill credits so
	// entries keep bubbling back and forth rather than all being rate-limited.
	for i := 0; i < 200; i++ {
		pIdx := i % len(patterns)
		tok := patterns[pIdx]
		name := names[pIdx]
		step++

		if i%2 == 0 {
			advanceTime(0.5)
		}

		// Snapshot matchCounts before Process.
		before := snapshotMatchCounts(s)
		beforeSampled := snapshotSampled(s)
		entryIdxBefore := findEntryIdx(s, tok)

		msg := message.NewMessage([]byte(fmt.Sprintf("step=%d pattern=%s", step, name)), nil, message.StatusInfo, 0)
		out := s.Process(msg, tok)

		// The matched entry (by its original index, which may have moved after
		// bubbling) must reflect exactly one additional matchCount.
		// We identify the entry by finding the one whose matchCount differs by 1
		// from the before snapshot for that entry index.
		after := snapshotMatchCounts(s)
		afterSampled := snapshotSampled(s)
		entryIdxAfter := findEntryIdx(s, tok)

		if entryIdxAfter < 0 {
			// Pattern was evicted during new-entry handling (only happens when
			// the pattern is brand new and there was no room). Not a bug.
			continue
		}

		// The matched entry's matchCount must have grown by exactly 1.
		wantMatchCount := before[entryIdxBefore] + 1
		gotMatchCount := after[entryIdxAfter]
		require.Equal(t, wantMatchCount, gotMatchCount,
			"step %d pattern %s: matched entry matchCount should be +1 (aliasing would write to wrong entry)",
			step, name)

		// The sampled field must be consistent with the allow/drop decision.
		if out != nil {
			// Message was allowed: sampled must have reset to 0.
			require.Equal(t, int64(0), afterSampled[entryIdxAfter],
				"step %d pattern %s: allowed message should reset sampled to 0 on the matched entry",
				step, name)
		} else {
			// Message was dropped: sampled must be beforeSampled+1.
			wantSampled := beforeSampled[entryIdxBefore] + 1
			require.Equal(t, wantSampled, afterSampled[entryIdxAfter],
				"step %d pattern %s: dropped message should increment sampled on matched entry (aliasing increments wrong entry)",
				step, name)
		}
	}
}

// snapshotMatchCounts returns a map from entry index to matchCount.
func snapshotMatchCounts(s *AdaptiveSampler) map[int]int64 {
	m := make(map[int]int64, len(s.entries))
	for i, e := range s.entries {
		m[i] = e.matchCount
	}
	return m
}

// snapshotSampled returns a map from entry index to sampled count.
func snapshotSampled(s *AdaptiveSampler) map[int]int64 {
	m := make(map[int]int64, len(s.entries))
	for i, e := range s.entries {
		m[i] = e.sampled
	}
	return m
}

// findEntryIdx returns the index of the entry matching tok, or -1 if not found.
func findEntryIdx(s *AdaptiveSampler, tok []Token) int {
	for i, e := range s.entries {
		if IsMatch(e.tokens, tok, s.config.MatchThreshold) {
			return i
		}
	}
	return -1
}
