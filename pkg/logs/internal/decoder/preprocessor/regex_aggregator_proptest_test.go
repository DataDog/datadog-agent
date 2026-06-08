// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"regexp"
	"testing"

	"pgregory.net/rapid"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the RegexAggregation surface declared in
// regex_aggregator.allium. Each test names the spec @guarantee it
// anchors so that drift in either direction is easy to spot during
// review.
//
// Anchoring unit tests for the same surface live in
// regex_aggregator_test.go.

// regexAggregatorCall bundles one process call's worth of inputs.
type regexAggregatorCall struct {
	content string
	label   Label
	tokens  []Token
}

// genRegexAggregatorCall produces a single process-call input. The
// content is sampled from a small alphabet that includes the
// literal prefix "START" with non-trivial probability, so a
// generated sequence exercises both pre-match and post-match
// branches reliably.
func genRegexAggregatorCall() *rapid.Generator[regexAggregatorCall] {
	return rapid.Custom(func(t *rapid.T) regexAggregatorCall {
		// Two-way choice: emit a content that starts with "START"
		// (matches the test pattern) or one that doesn't. This
		// biases the generated stream toward exercising the
		// pattern-boundary branches without making it impossible
		// to generate long no-match sequences.
		startsWithMarker := rapid.Bool().Draw(t, "startsWithMarker")
		tail := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abc 012")),
			0, 20,
		).Draw(t, "tail"))
		var content string
		if startsWithMarker {
			content = "START " + tail
		} else {
			content = tail
		}
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		nTokens := rapid.IntRange(0, 4).Draw(t, "nTokens")
		tokens := make([]Token, nTokens)
		for i := 0; i < nTokens; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
		}
		return regexAggregatorCall{content: content, label: label, tokens: tokens}
	})
}

// runCalls feeds a sequence of calls into an aggregator and
// collects all emitted message contents (Process + final Flush).
// Bytes are copied before subsequent calls invalidate them, in line
// with the Aggregator contract's ResultLifetime invariant.
func runCalls(ag *RegexAggregator, calls []regexAggregatorCall) []string {
	var out []string
	for _, c := range calls {
		emitted := ag.Process(newMessage(c.content), c.label, c.tokens)
		for _, e := range emitted {
			out = append(out, string(e.Msg.GetContent()))
		}
	}
	for _, e := range ag.Flush() {
		out = append(out, string(e.Msg.GetContent()))
	}
	return out
}

// TestRegexAggregator_LabelIgnored_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee LabelIgnored
//
// Across arbitrary generated call sequences, two aggregators fed
// the same content sequence but with all-different labels produce
// identical emitted content sequences. The label has no observable
// effect on aggregation, separator placement, or truncation
// decisions.
func TestRegexAggregator_LabelIgnored_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genRegexAggregatorCall(), 1, 12).Draw(t, "calls")

		callsA := make([]regexAggregatorCall, len(calls))
		callsB := make([]regexAggregatorCall, len(calls))
		for i, c := range calls {
			cA := c
			cA.label = startGroup
			callsA[i] = cA
			cB := c
			cB.label = aggregate
			callsB[i] = cB
		}

		agA := NewRegexAggregator(re, 100, false, status.NewInfoRegistry(), "multi_line")
		agB := NewRegexAggregator(re, 100, false, status.NewInfoRegistry(), "multi_line")
		outA := runCalls(agA, callsA)
		outB := runCalls(agB, callsB)

		if len(outA) != len(outB) {
			t.Fatalf("LabelIgnored violated: emission count %d (start_group) vs %d (aggregate)", len(outA), len(outB))
		}
		for i := range outA {
			if outA[i] != outB[i] {
				t.Fatalf("LabelIgnored violated at emission %d: %q (start_group) vs %q (aggregate)", i, outA[i], outB[i])
			}
		}
	})
}

// TestRegexAggregator_IsEmptyAfterFlush_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — After flush returns,
//	                                    buffer_empty is true
//	    @guarantee IsEmptyConsistency — is_empty reflects
//	                                     buffer_empty exactly
//
// Across arbitrary generated call sequences, calling Flush
// followed by IsEmpty always yields true.
func TestRegexAggregator_IsEmptyAfterFlush_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genRegexAggregatorCall(), 0, 10).Draw(t, "calls")
		ag := NewRegexAggregator(re, 100, false, status.NewInfoRegistry(), "multi_line")
		for _, c := range calls {
			ag.Process(newMessage(c.content), c.label, c.tokens)
		}
		ag.Flush()
		if !ag.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: is_empty false after flush")
		}
	})
}

// TestRegexAggregator_FlushIdempotentOnEmpty_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — A flush call on an already-
//	                                    empty buffer returns an
//	                                    empty sequence and changes
//	                                    no observable state.
//
// A second consecutive flush returns an empty sequence and
// leaves is_empty true.
func TestRegexAggregator_FlushIdempotentOnEmpty_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genRegexAggregatorCall(), 0, 10).Draw(t, "calls")
		ag := NewRegexAggregator(re, 100, false, status.NewInfoRegistry(), "multi_line")
		for _, c := range calls {
			ag.Process(newMessage(c.content), c.label, c.tokens)
		}
		ag.Flush()
		second := ag.Flush()
		if len(second) != 0 {
			t.Fatalf("FlushDrainsBuffer violated: second flush returned %d messages, expected 0", len(second))
		}
		if !ag.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: is_empty false after second flush")
		}
	})
}

// TestRegexAggregator_FlushEqualsTotalEmissions_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — flush emits any remaining
//	                                    buffered aggregate
//	    @guarantee PatternBoundary — pattern matches flush prior
//	                                  aggregates, lines accumulate
//	                                  otherwise
//
// At least one emission MUST come from Flush whenever any process
// call buffered content that no pattern boundary later flushed.
// Specifically: feed a sequence whose final line does NOT match
// the pattern; then Flush must produce exactly one emission (the
// tail aggregate). This pins the "buffer is fully drained by
// flush" property end-to-end.
//
// The lineLimit is sized large enough that the accumulated
// buffer cannot reach it within the generated input bounds —
// otherwise an overflow-during-Process would emit the tail
// aggregate via MidAggregateTruncation and leave the buffer
// empty for Flush, which would not satisfy this test's premise.
// genRegexAggregatorCall produces content up to "START " (6
// bytes) + 20 bytes (= 26 max) per call; with up to 10 calls
// plus 2-byte separators between them, the worst case is well
// under 1000 bytes.
func TestRegexAggregator_FlushEqualsTotalEmissions_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genRegexAggregatorCall(), 1, 10).Draw(t, "calls")
		// Force the final line to be non-matching content so the
		// buffer is non-empty when Flush is called.
		calls[len(calls)-1].content = "trailing non-match"
		ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")
		for _, c := range calls {
			ag.Process(newMessage(c.content), c.label, c.tokens)
		}
		flushed := ag.Flush()
		if len(flushed) != 1 {
			t.Fatalf("FlushDrainsBuffer violated: expected exactly 1 flush emission, got %d", len(flushed))
		}
		if !ag.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: is_empty false after flush")
		}
	})
}

// regexEmission captures one emitted message's observable state,
// deep-copied to survive the RegexAggregator's ResultLifetime
// reuse of its collected slice across subsequent Process calls.
type regexEmission struct {
	content     []byte
	isTruncated bool
	tags        []string
}

func captureRegexEmissions(emitted []AggregatedMessageWithTokens) []regexEmission {
	out := make([]regexEmission, len(emitted))
	for i, e := range emitted {
		out[i] = regexEmission{
			content:     append([]byte(nil), e.Msg.GetContent()...),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			tags:        append([]string(nil), e.Msg.ParsingExtra.Tags...),
		}
	}
	return out
}

// TestRegexAggregator_PassThroughUnderThreshold_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee ByteConservation — under-threshold accumulation
//	                                   emits with no markers and
//	                                   IsTruncated false.
//
// A pattern-bounded group whose accumulated content stays under
// line_limit emits with no truncation markers and IsTruncated=false.
// Together with UpstreamFlagIgnored, this confirms the only path
// to a truncated emission is buffer overflow.
func TestRegexAggregator_PassThroughUnderThreshold_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		nContinuations := rapid.IntRange(0, 3).Draw(t, "nContinuations")
		ag := NewRegexAggregator(re, 200, false, status.NewInfoRegistry(), "multi_line")

		ag.Process(newMessage("START first"), startGroup, nil)
		for i := 0; i < nContinuations; i++ {
			line := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef")),
				1, 8,
			).Draw(t, "cont"))
			ag.Process(newMessage(line), aggregate, nil)
		}
		emitted := captureRegexEmissions(ag.Process(newMessage("START second"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission from pattern-boundary, got %d", len(emitted))
		}
		e := emitted[0]
		marker := message.TruncatedFlag
		if bytes.Contains(e.content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: marker found in under-threshold emission %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("PassThroughUnderThreshold violated: IsTruncated=true on under-threshold emission %q", e.content)
		}
	})
}

// TestRegexAggregator_MidAggregateTruncation_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MidAggregateTruncation — when buffered aggregate
//	                                         reaches or exceeds
//	                                         line_limit, the buffer
//	                                         is flushed mid-stream
//	                                         with the truncation
//	                                         marker appended.
//
// A pattern-bounded group whose accumulated content crosses
// line_limit during a Process call emits the buffered content
// with the tail marker appended and IsTruncated=true on the SAME
// process call that caused the overflow.
func TestRegexAggregator_MidAggregateTruncation_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		// Continuation longer than lineLimit alone forces overflow
		// when added to "START a" + separator.
		contLen := lineLimit + 5 + rapid.IntRange(0, 20).Draw(t, "extra")
		contBytes := bytes.Repeat([]byte("x"), contLen)

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START a"), startGroup, nil)
		emitted := captureRegexEmissions(ag.Process(newMessage(string(contBytes)), aggregate, nil))

		if len(emitted) == 0 {
			t.Fatal("MidAggregateTruncation violated: expected emission after overflow, got none")
		}
		e := emitted[len(emitted)-1]
		marker := message.TruncatedFlag
		if !bytes.HasSuffix(e.content, marker) {
			t.Fatalf("MidAggregateTruncation violated: no tail marker on overflow emission %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("MidAggregateTruncation violated: IsTruncated=false on overflow emission")
		}
	})
}

// TestRegexAggregator_HeadMarkerOnCarryover_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MidAggregateTruncation — the continuation-marker
//	                                         carry is consumed by
//	                                         the next post-match
//	                                         non-matching process
//	                                         call, prepending the
//	                                         truncation marker to
//	                                         that call's buffered
//	                                         content.
//
// After overflow sets should_truncate, the next non-matching
// process call accumulates with the head marker prepended; the
// eventual emission (here via Flush) begins with the marker.
func TestRegexAggregator_HeadMarkerOnCarryover_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		// lineLimit large enough that post-carry accumulation
		// (head marker 15 + short content) stays under limit.
		lineLimit := rapid.IntRange(25, 60).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)
		shortBytes := bytes.Repeat([]byte("a"), rapid.IntRange(1, 3).Draw(t, "shortLen"))

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START a"), startGroup, nil)
		ag.Process(newMessage(string(overflowBytes)), aggregate, nil) // overflow → emit with tail
		ag.Process(newMessage(string(shortBytes)), aggregate, nil)    // accumulate with head marker
		flushed := captureRegexEmissions(ag.Flush())                  // emits the carry-marked content

		if len(flushed) != 1 {
			t.Fatalf("HeadMarkerOnCarryover violated: expected 1 flush emission, got %d", len(flushed))
		}
		e := flushed[0]
		marker := message.TruncatedFlag
		if !bytes.HasPrefix(e.content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on emission after carry; content %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestRegexAggregator_CarryoverConsumed_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MidAggregateTruncation — the continuation carry
//	                                         is consumed by the
//	                                         next non-matching call
//	                                         and does NOT propagate
//	                                         further.
//
// After overflow (carry set) + non-matching consume (head marker
// written) + flush, a fresh pattern-bounded group emits with NO
// markers — the carry was consumed and not propagated.
func TestRegexAggregator_CarryoverConsumed_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(25, 60).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)
		shortBytes := bytes.Repeat([]byte("a"), rapid.IntRange(1, 3).Draw(t, "shortLen"))
		freshBytes := bytes.Repeat([]byte("b"), rapid.IntRange(1, 3).Draw(t, "freshLen"))

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START a"), startGroup, nil)
		ag.Process(newMessage(string(overflowBytes)), aggregate, nil) // emission 1: tail marker, carry set
		ag.Process(newMessage(string(shortBytes)), aggregate, nil)    // accumulates with head marker
		emission2 := captureRegexEmissions(ag.Flush())                // emission 2: head marker
		// Now carry is consumed (should_truncate is false). Start fresh.
		ag.Process(newMessage("START second"), startGroup, nil)
		ag.Process(newMessage(string(freshBytes)), aggregate, nil)
		emission3 := captureRegexEmissions(ag.Flush()) // emission 3: no markers

		marker := message.TruncatedFlag

		if len(emission2) != 1 {
			t.Fatalf("CarryoverConsumed precondition: expected 1 emission from first flush, got %d", len(emission2))
		}
		if !bytes.HasPrefix(emission2[0].content, marker) {
			t.Fatalf("CarryoverConsumed precondition: first flush emission missing head marker; got %q", emission2[0].content)
		}

		if len(emission3) != 1 {
			t.Fatalf("CarryoverConsumed violated: expected 1 emission from second flush, got %d", len(emission3))
		}
		e := emission3[0]
		if bytes.Contains(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: fresh emission contains a marker — carry not consumed; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("CarryoverConsumed violated: fresh emission IsTruncated=true on clean accumulation; content %q", e.content)
		}
	})
}

// TestRegexAggregator_UpstreamFlagIgnored_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee UpstreamFlagIgnored — input ParsingExtra.IsTruncated
//	                                      is NOT read and does NOT
//	                                      influence any truncation
//	                                      decision; deliberate
//	                                      DEVIATION from the
//	                                      Truncatable contract.
//
// LOAD-BEARING for the refactor safety net. Inputs with upstream
// IsTruncated=true that do NOT cause buffer overflow MUST emit
// with IsTruncated=false. A refactor that began honouring the
// upstream flag here would break this test — exactly the
// behaviour change we want to prevent inadvertently.
func TestRegexAggregator_UpstreamFlagIgnored_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 1000 // large, no overflow
		shortBytes := bytes.Repeat([]byte("a"), rapid.IntRange(1, 8).Draw(t, "shortLen"))

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START first"), startGroup, nil)
		// All continuation messages arrive upstream-flagged but
		// well under the buffer limit.
		nFlagged := rapid.IntRange(1, 4).Draw(t, "nFlagged")
		for i := 0; i < nFlagged; i++ {
			msg := newMessage(string(shortBytes))
			msg.ParsingExtra.IsTruncated = true
			ag.Process(msg, aggregate, nil)
		}
		// Pattern boundary triggers emission of the prior group.
		emitted := captureRegexEmissions(ag.Process(newMessage("START second"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("UpstreamFlagIgnored: expected 1 emission from boundary, got %d", len(emitted))
		}
		e := emitted[0]
		marker := message.TruncatedFlag
		if bytes.Contains(e.content, marker) {
			t.Fatalf("UpstreamFlagIgnored violated: emission contains marker — upstream flag honoured; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("UpstreamFlagIgnored violated: emission IsTruncated=true without overflow — upstream flag propagated; content %q", e.content)
		}
	})
}

// TestRegexAggregator_TruncationTagging_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee TruncationTagging — when is_buffer_truncated is
//	                                    true AND
//	                                    logs_config.tag_truncated_logs
//	                                    is true, a truncation-reason
//	                                    tag identifying
//	                                    "multiline_regex" is appended.
//
// With the global config enabled, every emission whose
// IsTruncated is true carries the truncation-reason tag with
// value "multiline_regex" (matching MultiLineHandler's tag value,
// distinct from the "single_line" reason used by per-line
// aggregators). Emissions with IsTruncated=false do not carry it.
func TestRegexAggregator_TruncationTagging_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_truncated_logs", true, pkgconfigmodel.SourceAgentRuntime)
	expectedTag := message.TruncatedReasonTag("multiline_regex")
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START a"), startGroup, nil)
		emitted := captureRegexEmissions(ag.Process(newMessage(string(overflowBytes)), aggregate, nil))

		for i, e := range emitted {
			if e.isTruncated {
				hasTag := false
				for _, tag := range e.tags {
					if tag == expectedTag {
						hasTag = true
						break
					}
				}
				if !hasTag {
					t.Fatalf("TruncationTagging violated at emission %d: IsTruncated=true but no %q in tags=%v", i, expectedTag, e.tags)
				}
			} else {
				for _, tag := range e.tags {
					if tag == expectedTag {
						t.Fatalf("TruncationTagging violated at emission %d: IsTruncated=false but tags contain %q (tags=%v)", i, expectedTag, e.tags)
					}
				}
			}
		}
	})
}

// TestRegexAggregator_TagDisabledNoTag_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee TruncationTagging — when
//	                                    logs_config.tag_truncated_logs
//	                                    is false, NO truncation-reason
//	                                    tag is added regardless of
//	                                    is_buffer_truncated state.
//
// With the global config disabled, no emission carries the
// "multiline_regex" tag, even when buffer overflow has set
// IsTruncated=true.
func TestRegexAggregator_TagDisabledNoTag_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_truncated_logs", false, pkgconfigmodel.SourceAgentRuntime)
	suppressedTag := message.TruncatedReasonTag("multiline_regex")
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)

		ag := NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START a"), startGroup, nil)
		emitted := captureRegexEmissions(ag.Process(newMessage(string(overflowBytes)), aggregate, nil))

		for i, e := range emitted {
			for _, tag := range e.tags {
				if tag == suppressedTag {
					t.Fatalf("TruncationTagging (disabled) violated at emission %d: found %q in tags=%v", i, suppressedTag, e.tags)
				}
			}
		}
	})
}

// TestRegexAggregator_MultiLineTagging_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MultiLineTagging — when lines_combined > 1 at
//	                                   emission time AND
//	                                   logs_config.tag_multi_line_logs
//	                                   is true, the multi-line
//	                                   source tag with value
//	                                   multi_line_tag_value is added.
//
// A pattern-bounded group containing 2+ input lines emits with
// the multi-line source tag (value "multi_line" per the
// constructor argument).
func TestRegexAggregator_MultiLineTagging_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)
	multiLineTag := message.MultiLineSourceTag("multi_line")
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		nContinuations := rapid.IntRange(1, 4).Draw(t, "nContinuations")
		ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

		ag.Process(newMessage("START first"), startGroup, nil)
		for i := 0; i < nContinuations; i++ {
			ag.Process(newMessage("more"), aggregate, nil)
		}
		// Pattern boundary triggers emission of the multi-line group.
		emitted := captureRegexEmissions(ag.Process(newMessage("START second"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission from boundary, got %d", len(emitted))
		}
		e := emitted[0]
		hasTag := false
		for _, tag := range e.tags {
			if tag == multiLineTag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Fatalf("MultiLineTagging violated: %d-line group emission missing tag %q; tags=%v", 1+nContinuations, multiLineTag, e.tags)
		}
	})
}

// TestRegexAggregator_TokensFromAggregateLeader_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee TokensFromAggregateLeader — emitted tokens are
//	                                            the tokens passed
//	                                            alongside the FIRST
//	                                            line of the
//	                                            aggregate.
//
// LOAD-BEARING for the refactor safety net. The aggregator
// stores firstLineTokens on the leader call and uses those at
// flush time, NOT the last-contributor's tokens. If a refactor
// swapped this to last-tokens or recomputed, downstream tagging
// would silently drift.
func TestRegexAggregator_TokensFromAggregateLeader_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)

	rapid.Check(t, func(t *rapid.T) {
		leaderTokens := []Token{Token(1), Token(2), Token(3)}
		continuationTokens := []Token{Token(7), Token(8)}

		ag := NewRegexAggregator(re, 200, false, status.NewInfoRegistry(), "multi_line")
		ag.Process(newMessage("START leader"), startGroup, leaderTokens)
		ag.Process(newMessage("continuation"), aggregate, continuationTokens)
		// Boundary triggers emission of the prior group.
		emitted := ag.Process(newMessage("START next"), startGroup, []Token{Token(0)})

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission from boundary, got %d", len(emitted))
		}
		gotTokens := emitted[0].Tokens
		if len(gotTokens) != len(leaderTokens) {
			t.Fatalf("TokensFromAggregateLeader violated: emission has %d tokens, leader had %d", len(gotTokens), len(leaderTokens))
		}
		for i := range leaderTokens {
			if gotTokens[i] != leaderTokens[i] {
				t.Fatalf("TokensFromAggregateLeader violated: emission token[%d]=%v, expected leader token[%d]=%v (or did the aggregator forward continuation tokens %v?)", i, gotTokens[i], i, leaderTokens[i], continuationTokens)
			}
		}
	})
}

// TestRegexAggregator_PreMatchSinglePass_Property anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee PreMatchSinglePass — before pattern_matched_once
//	                                     becomes true, each call's
//	                                     input is the sole occupant
//	                                     of the buffer.
//
// With a regex that never matches the generated content, a
// sequence of N process calls + a final flush produces exactly N
// emissions — each input line is emitted individually rather
// than accumulated.
func TestRegexAggregator_PreMatchSinglePass_Property(t *testing.T) {
	// Pattern requiring a Z prefix; combined with content drawn
	// from a Z-free alphabet, never matches.
	re := regexp.MustCompile("^Z")

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		ag := NewRegexAggregator(re, 200, false, status.NewInfoRegistry(), "multi_line")

		emittedTotal := 0
		for i := 0; i < n; i++ {
			line := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef0123")),
				1, 20,
			).Draw(t, "line"))
			emittedTotal += len(ag.Process(newMessage(line), aggregate, nil))
		}
		emittedTotal += len(ag.Flush())

		if emittedTotal != n {
			t.Fatalf("PreMatchSinglePass violated: %d inputs produced %d emissions, expected %d", n, emittedTotal, n)
		}
	})
}
