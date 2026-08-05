// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Property tests for the AdaptiveSampling surface declared in
// adaptive_sampler.allium. Each test names the spec @guarantee or
// @invariant it anchors so that drift in either direction is easy
// to spot during review.
//
// Anchoring unit tests for the same surface live in
// adaptive_sampler_test.go and sampler_test.go.

// samplerCall bundles one process call's worth of input.
type samplerCall struct {
	content         []byte
	tokens          []Token
	inputTags       []string
	advanceMs       int64
	isImportantCall bool // when true, draw an ERROR-tagged token sequence
}

// genSamplerCall produces one call. The token sequences are drawn
// from a small pool of structurally-distinct patterns plus the
// option to embed an ERROR token (driving the is_important
// predicate true) so generated sequences exercise the
// ImportantLogBypass branch alongside the pattern-table rules.
func genSamplerCall() *rapid.Generator[samplerCall] {
	return rapid.Custom(func(t *rapid.T) samplerCall {
		content := rapid.SliceOfN(rapid.Byte(), 0, 40).Draw(t, "content")
		patternChoice := rapid.IntRange(0, 2).Draw(t, "patternChoice")
		var tokens []Token
		switch patternChoice {
		case 0:
			tokens = tokenize("2024-01-15 10:30:45 INFO request id=123")
		case 1:
			tokens = tokenize("metric cpu_usage=45.67 host=web-01")
		default:
			tokens = tokenize("http GET /api/users 200 42ms")
		}
		isImp := rapid.Bool().Draw(t, "isImportant")
		if isImp {
			// Replace with an ERROR-tagged token sequence to drive
			// the is_important predicate true.
			tokens = tokenize("ERROR something went wrong with the operation")
		}
		// Optional input tags drawn from a small pool — exercises
		// TagAugmentationOnly's "preserve existing tags" half.
		tagCount := rapid.IntRange(0, 2).Draw(t, "tagCount")
		tags := make([]string, tagCount)
		for i := 0; i < tagCount; i++ {
			tags[i] = rapid.SampledFrom([]string{"env:prod", "service:web", "team:platform"}).Draw(t, "tag")
		}
		advance := rapid.IntRange(0, 5000).Draw(t, "advanceMs")
		return samplerCall{
			content:         content,
			tokens:          tokens,
			inputTags:       tags,
			advanceMs:       int64(advance),
			isImportantCall: isImp,
		}
	})
}

// newSamplerForProptest builds a sampler with a deterministic
// clock injected via the now field. The clock advances on each
// call by the call's advanceMs amount; this gives rapid control
// over the credit-refill behaviour without real time.
func newSamplerForProptest(burst, rateLimit float64, protect bool) *AdaptiveSampler {
	s := NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:          10,
		RateLimit:            rateLimit,
		BurstSize:            burst,
		MatchThreshold:       0.9,
		ProtectImportantLogs: protect,
	}, "proptest", 0)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }
	return s
}

// runOneCall feeds one generated call into the sampler and returns
// whether the message was emitted (non-nil result), the emitted
// message's tag set (or nil if dropped), and a copy of the
// content bytes (or nil if dropped).
//
// The clock advance for the call is applied BEFORE the Process
// invocation, modelling time elapsed since the last call.
func runOneCall(s *AdaptiveSampler, t0 time.Time, totalElapsed *time.Duration, call samplerCall) (emitted bool, tags []string, content []byte) {
	*totalElapsed += time.Duration(call.advanceMs) * time.Millisecond
	elapsed := *totalElapsed
	s.now = func() time.Time { return t0.Add(elapsed) }
	msg := message.NewMessage(append([]byte(nil), call.content...), nil, message.StatusInfo, 0)
	msg.ParsingExtra.Tags = append([]string(nil), call.inputTags...)
	out := s.Process(msg, call.tokens)
	if out == nil {
		return false, nil, nil
	}
	return true, append([]string(nil), out.ParsingExtra.Tags...), append([]byte(nil), out.GetContent()...)
}

// TestAdaptiveSampler_Determinism_Property anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee Determinism
//
// Two samplers with identical configuration, fed identical
// (call sequence, clock schedule) inputs, produce identical
// emission decisions and identical emitted-tag sets.
func TestAdaptiveSampler_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genSamplerCall(), 1, 12).Draw(t, "calls")
		burst := rapid.Float64Range(1.0, 5.0).Draw(t, "burst")
		rateLimit := rapid.Float64Range(0, 3).Draw(t, "rateLimit")
		protect := rapid.Bool().Draw(t, "protect")

		runSampler := func() (decisions []bool, tagSets [][]string) {
			s := newSamplerForProptest(burst, rateLimit, protect)
			t0 := time.Now()
			s.now = func() time.Time { return t0 }
			var elapsed time.Duration
			for _, c := range calls {
				emitted, tags, _ := runOneCall(s, t0, &elapsed, c)
				decisions = append(decisions, emitted)
				tagSets = append(tagSets, tags)
			}
			return
		}

		decA, tagsA := runSampler()
		decB, tagsB := runSampler()

		if len(decA) != len(decB) {
			t.Fatalf("Determinism violated: emission counts %d vs %d", len(decA), len(decB))
		}
		for i := range decA {
			if decA[i] != decB[i] {
				t.Fatalf("Determinism violated at call %d: emit decision %v vs %v", i, decA[i], decB[i])
			}
			if !stringSlicesEqualAsSet(tagsA[i], tagsB[i]) {
				t.Fatalf("Determinism violated at call %d: tag sets %v vs %v", i, tagsA[i], tagsB[i])
			}
		}
	})
}

// TestAdaptiveSampler_ContentBytePassthrough_Property anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant ContentBytePassthrough — when process returns a
//	                                         Message, the returned
//	                                         Message's content
//	                                         bytes equal the input
//	                                         msg's content bytes.
//
// For arbitrary inputs, every non-null emission preserves the
// content bytes exactly.
func TestAdaptiveSampler_ContentBytePassthrough_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genSamplerCall(), 1, 8).Draw(t, "calls")
		s := newSamplerForProptest(3.0, 1.0, true)
		t0 := time.Now()
		s.now = func() time.Time { return t0 }
		var elapsed time.Duration

		for _, c := range calls {
			emitted, _, content := runOneCall(s, t0, &elapsed, c)
			if emitted && !bytes.Equal(content, c.content) {
				t.Fatalf("ContentBytePassthrough violated: input %q → output %q", c.content, content)
			}
		}
	})
}

// TestAdaptiveSampler_TagAugmentationOnly_Property anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant TagAugmentationOnly — returned message's tags
//	                                      are a superset of the
//	                                      input msg's tags; the
//	                                      only tag the sampler may
//	                                      append is
//	                                      "adaptive_sampler_sampled_count:N".
//
// Across arbitrary inputs: every emitted message's tag set is a
// superset of the input's tag set, and any added tags are
// adaptive_sampler_sampled_count tags.
func TestAdaptiveSampler_TagAugmentationOnly_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genSamplerCall(), 1, 8).Draw(t, "calls")
		s := newSamplerForProptest(2.0, 0.5, false)
		t0 := time.Now()
		s.now = func() time.Time { return t0 }
		var elapsed time.Duration

		for _, c := range calls {
			emitted, outTags, _ := runOneCall(s, t0, &elapsed, c)
			if !emitted {
				continue
			}
			for _, inTag := range c.inputTags {
				if !containsString(outTags, inTag) {
					t.Fatalf("TagAugmentationOnly violated: input tag %q missing from emitted tags %v", inTag, outTags)
				}
			}
			for _, outTag := range outTags {
				if containsString(c.inputTags, outTag) {
					continue
				}
				if !strings.HasPrefix(outTag, "adaptive_sampler_sampled_count:") {
					t.Fatalf("TagAugmentationOnly violated: emitted tag %q is neither an input tag nor an adaptive_sampler_sampled_count tag", outTag)
				}
			}
		}
	})
}

// TestAdaptiveSampler_FlushAlwaysNil_Property anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee FlushNoop — flush always returns null.
//
// After any sequence of Process calls, Flush returns nil.
// AdaptiveSampler has no buffer to drain.
func TestAdaptiveSampler_FlushAlwaysNil_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genSamplerCall(), 0, 10).Draw(t, "calls")
		s := newSamplerForProptest(3.0, 1.0, true)
		t0 := time.Now()
		s.now = func() time.Time { return t0 }
		var elapsed time.Duration
		for _, c := range calls {
			runOneCall(s, t0, &elapsed, c)
		}
		if got := s.Flush(); got != nil {
			t.Fatalf("FlushNoop violated: flush returned non-nil message (content=%q)", got.GetContent())
		}
	})
}

// TestAdaptiveSampler_ImportantLogProtection_Property anchors:
//
//	surface AdaptiveSampling (adaptive_sampler.allium)
//	    @guarantee ImportantLogProtection — when protection is
//	                                         enabled, important
//	                                         logs always emit
//	                                         regardless of pattern
//	                                         table state.
//
// With protect_important_logs enabled, every call carrying an
// important token sequence emits. Pattern entry count never
// changes for important calls (the bypass is non-disruptive).
func TestAdaptiveSampler_ImportantLogProtection_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genSamplerCall(), 1, 8).Draw(t, "calls")
		s := newSamplerForProptest(1.0, 0, true) // tight: burst=1 no refill
		t0 := time.Now()
		s.now = func() time.Time { return t0 }
		var elapsed time.Duration

		for i, c := range calls {
			entryCountBefore := len(s.entries)
			emitted, _, _ := runOneCall(s, t0, &elapsed, c)
			if c.isImportantCall {
				if !emitted {
					t.Fatalf("ImportantLogProtection violated at call %d: important log dropped", i)
				}
				if len(s.entries) != entryCountBefore {
					t.Fatalf("ImportantLogProtection violated at call %d: pattern table size changed (%d → %d) on important log",
						i, entryCountBefore, len(s.entries))
				}
			}
		}
	})
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func stringSlicesEqualAsSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !containsString(b, x) {
			return false
		}
	}
	return true
}
