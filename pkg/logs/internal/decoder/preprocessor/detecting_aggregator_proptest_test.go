// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the DetectingAggregation surface declared in
// detecting_aggregator.allium. Each test names the spec
// @guarantee it anchors so that drift in either direction is easy
// to spot during review.
//
// Anchoring unit tests for the same surface live in
// detecting_aggregator_test.go and aggregator_test.go.

// detectingCall bundles one process call's worth of inputs.
type detectingCall struct {
	content string
	label   Label
	tokens  []Token
}

func genDetectingCall() *rapid.Generator[detectingCall] {
	return rapid.Custom(func(t *rapid.T) detectingCall {
		content := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abc 012")),
			0, 30,
		).Draw(t, "content"))
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		nTokens := rapid.IntRange(0, 4).Draw(t, "nTokens")
		tokens := make([]Token, nTokens)
		for i := 0; i < nTokens; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
		}
		return detectingCall{content: content, label: label, tokens: tokens}
	})
}

// detectingEmission captures one emitted message's observable
// state. Used to compare across runs for determinism tests.
type detectingEmission struct {
	content     string
	isTruncated bool
	tagsLen     int
	tokensLen   int
}

// runDetectingCalls feeds a sequence of calls into a freshly
// constructed DetectingAggregator and returns the observable state
// of each emission, in arrival order, including a final flush.
func runDetectingCalls(lineLimit int, tagTruncated bool, calls []detectingCall) []detectingEmission {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, tagTruncated, false)
	var out []detectingEmission
	for _, c := range calls {
		emitted := ag.Process(newMessage(c.content), c.label, c.tokens)
		for _, e := range emitted {
			out = append(out, detectingEmission{
				content:     string(e.Msg.GetContent()),
				isTruncated: e.Msg.ParsingExtra.IsTruncated,
				tagsLen:     len(e.Msg.ParsingExtra.Tags),
				tokensLen:   len(e.Tokens),
			})
		}
	}
	for _, e := range ag.Flush() {
		out = append(out, detectingEmission{
			content:     string(e.Msg.GetContent()),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			tagsLen:     len(e.Msg.ParsingExtra.Tags),
			tokensLen:   len(e.Tokens),
		})
	}
	return out
}

// TestDetectingAggregator_NoLineCombination_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee NoLineCombination — Every emitted
//	                                    AggregatedMessageWithTokens
//	                                    carries the content of
//	                                    exactly one input message.
//
// Across arbitrary call sequences, the total number of emissions
// (process across all calls + final flush) equals the total
// number of input process calls. No input is dropped; no input
// is folded into another's emission.
func TestDetectingAggregator_NoLineCombination_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genDetectingCall(), 1, 12).Draw(t, "calls")
		out := runDetectingCalls(100, false, calls)
		if len(out) != len(calls) {
			t.Fatalf("NoLineCombination violated: %d inputs produced %d emissions", len(calls), len(out))
		}
	})
}

// TestDetectingAggregator_Determinism_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee Determinism — process, flush and is_empty are
//	                              pure functions of the
//	                              aggregator's current state plus
//	                              their inputs.
//
// Two aggregators of identical configuration, fed identical
// call sequences, produce equal observed emission sequences. The
// per-call label dispatch and the rolling truncation carry are
// both deterministic functions of inputs.
func TestDetectingAggregator_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genDetectingCall(), 1, 12).Draw(t, "calls")
		lineLimit := rapid.IntRange(1, 100).Draw(t, "lineLimit")

		outA := runDetectingCalls(lineLimit, false, calls)
		outB := runDetectingCalls(lineLimit, false, calls)

		if len(outA) != len(outB) {
			t.Fatalf("Determinism violated: emission counts %d vs %d", len(outA), len(outB))
		}
		for i := range outA {
			if outA[i] != outB[i] {
				t.Fatalf("Determinism violated at emission %d: %+v vs %+v", i, outA[i], outB[i])
			}
		}
	})
}

// TestDetectingAggregator_FlushClearsState_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — After flush returns,
//	                                    pending_message_present is
//	                                    false.
//	    @guarantee IsEmptyConsistency
//
// Across arbitrary call sequences, flush followed by is_empty
// always yields true. Calling flush twice in a row produces an
// empty second flush.
func TestDetectingAggregator_FlushClearsState_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genDetectingCall(), 0, 12).Draw(t, "calls")
		ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)
		for _, c := range calls {
			ag.Process(newMessage(c.content), c.label, c.tokens)
		}
		ag.Flush()
		if !ag.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: is_empty false after flush")
		}
		second := ag.Flush()
		if len(second) != 0 {
			t.Fatalf("FlushDrainsBuffer violated: second flush returned %d messages, expected 0", len(second))
		}
	})
}

// makeDetectingMessage builds a *message.Message with the given
// content and upstream IsTruncated flag. Mirrors the inputs that
// would arrive from the framer.
func makeDetectingMessage(content []byte, upstreamIsTruncated bool) *message.Message {
	m := message.NewMessage(append([]byte(nil), content...), nil, message.StatusInfo, 0)
	m.RawDataLen = len(content)
	m.ParsingExtra.IsTruncated = upstreamIsTruncated
	return m
}

// detectingCapture is a deep-copied snapshot of one emission's
// observable state. The DetectingAggregator reuses its collected
// slice's backing array across Process calls (per the
// ResultLifetime guarantee), so tests that inspect emissions from
// an earlier call after a later call must copy out the bytes,
// flag, and tags before the next Process call perturbs the
// backing storage.
type detectingCapture struct {
	content     []byte
	isTruncated bool
	tags        []string
}

func captureDetectingEmissions(emitted []AggregatedMessageWithTokens) []detectingCapture {
	out := make([]detectingCapture, len(emitted))
	for i, e := range emitted {
		out[i] = detectingCapture{
			content:     append([]byte(nil), e.Msg.GetContent()...),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			tags:        append([]string(nil), e.Msg.ParsingExtra.Tags...),
		}
	}
	return out
}

// TestDetectingAggregator_PassThroughUnderThreshold_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PassThroughUnderThreshold — (inherited from
//	                                            Truncatable) under-
//	                                            threshold + no carry
//	                                            + no upstream flag ⇒
//	                                            no markers,
//	                                            IsTruncated false.
//
// A single process call with label=aggregate on a fresh aggregator
// (no pending message) whose content is strictly under
// maxContentSize and whose ParsingExtra.IsTruncated is false emits
// trim-spaced content with no truncation markers and
// IsTruncated=false.
func TestDetectingAggregator_PassThroughUnderThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(50, 200).Draw(t, "lineLimit")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef 0123\t")),
			0, lineLimit/2,
		).Draw(t, "raw")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		emitted := ag.Process(makeDetectingMessage(raw, false), aggregate, nil)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		e := emitted[0].Msg

		marker := message.TruncatedFlag
		if bytes.HasPrefix(e.GetContent(), marker) {
			t.Fatalf("PassThroughUnderThreshold violated: head marker present in %q", e.GetContent())
		}
		if bytes.HasSuffix(e.GetContent(), marker) {
			t.Fatalf("PassThroughUnderThreshold violated: tail marker present in %q", e.GetContent())
		}
		if e.ParsingExtra.IsTruncated {
			t.Fatal("PassThroughUnderThreshold violated: IsTruncated=true on under-threshold non-upstream input")
		}
		expected := bytes.TrimSpace(raw)
		if !bytes.Equal(e.GetContent(), expected) {
			t.Fatalf("PassThroughUnderThreshold violated: content %q != trimmed input %q", e.GetContent(), expected)
		}
	})
}

// TestDetectingAggregator_TailMarkerOnOverThreshold_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee TailMarkerOnOverThreshold — (inherited from
//	                                            Truncatable) content
//	                                            over threshold ⇒
//	                                            tail marker,
//	                                            IsTruncated true.
//
// A single process call with label=aggregate on a fresh aggregator
// whose content has byte length strictly greater than
// maxContentSize emits content ending with the truncation marker
// and IsTruncated=true.
func TestDetectingAggregator_TailMarkerOnOverThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "raw")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		emitted := ag.Process(makeDetectingMessage(raw, false), aggregate, nil)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		e := emitted[0].Msg

		marker := message.TruncatedFlag
		if !bytes.HasSuffix(e.GetContent(), marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: no tail marker in %q", e.GetContent())
		}
		if bytes.HasPrefix(e.GetContent(), marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: unexpected head marker on fresh aggregator in %q", e.GetContent())
		}
		if !e.ParsingExtra.IsTruncated {
			t.Fatal("TailMarkerOnOverThreshold violated: IsTruncated=false on over-threshold input")
		}
	})
}

// TestDetectingAggregator_HeadMarkerOnCarryover_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee HeadMarkerOnCarryover — (inherited from
//	                                        Truncatable) prior carry
//	                                        ⇒ head marker on next
//	                                        emission, IsTruncated
//	                                        true.
//
// After an aggregate-labelled emission that truncates (setting
// carry=true), the next aggregate-labelled emission begins with
// the truncation marker and IsTruncated=true, even when its own
// content is under threshold and unflagged.
func TestDetectingAggregator_HeadMarkerOnCarryover_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		first := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "first")
		second := rapid.SliceOfN(
			rapid.SampledFrom([]byte("xyzwv98765")),
			1, lineLimit/2+1,
		).Draw(t, "second")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		ag.Process(makeDetectingMessage(first, false), aggregate, nil)
		emitted := ag.Process(makeDetectingMessage(second, false), aggregate, nil)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission on call 2, got %d", len(emitted))
		}
		e := emitted[0].Msg

		marker := message.TruncatedFlag
		if !bytes.HasPrefix(e.GetContent(), marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on second emission %q", e.GetContent())
		}
		if bytes.HasSuffix(e.GetContent(), marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: unexpected tail marker on under-threshold second emission %q", e.GetContent())
		}
		if !e.ParsingExtra.IsTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestDetectingAggregator_CarryoverConsumed_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee CarryoverConsumed — (inherited from Truncatable)
//	                                    carry is determined solely
//	                                    by current input.
//
// After an emission that sets carry (over-threshold input), a
// next emission under threshold and unflagged consumes the carry
// (emitting with head marker) and the THIRD emission must NOT
// carry the marker — verifying carry reset on emission 2.
func TestDetectingAggregator_CarryoverConsumed_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		first := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "first")
		second := rapid.SliceOfN(
			rapid.SampledFrom([]byte("xyzwv98765")),
			1, lineLimit/2+1,
		).Draw(t, "second")
		third := rapid.SliceOfN(
			rapid.SampledFrom([]byte("pqrstu43210")),
			1, lineLimit/2+1,
		).Draw(t, "third")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		ag.Process(makeDetectingMessage(first, false), aggregate, nil)
		emittedB := captureDetectingEmissions(ag.Process(makeDetectingMessage(second, false), aggregate, nil))
		emittedC := captureDetectingEmissions(ag.Process(makeDetectingMessage(third, false), aggregate, nil))
		if len(emittedB) != 1 || len(emittedC) != 1 {
			t.Fatalf("expected 1 emission each on calls 2 and 3, got %d / %d", len(emittedB), len(emittedC))
		}

		marker := message.TruncatedFlag

		// Precondition: emission 2 should carry the head marker.
		if !bytes.HasPrefix(emittedB[0].content, marker) {
			t.Fatalf("CarryoverConsumed precondition: emission 2 should have head marker; got %q", emittedB[0].content)
		}

		// The point: emission 3 must NOT carry a head marker.
		e := emittedC[0]
		if bytes.HasPrefix(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 carries head marker after emission 2 should have consumed it; content %q", e.content)
		}
		if bytes.HasSuffix(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 has tail marker on under-threshold input; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatal("CarryoverConsumed violated: emission 3 IsTruncated=true on clean under-threshold input")
		}
	})
}

// TestDetectingAggregator_UpstreamFlagPropagation_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee UpstreamFlagPropagation — (inherited from
//	                                          Truncatable) upstream
//	                                          IsTruncated true
//	                                          behaves like over-
//	                                          threshold.
//
// On a fresh aggregator, an aggregate-labelled under-threshold
// input whose ParsingExtra.IsTruncated=true emits with the tail
// marker and IsTruncated=true. The next aggregate-labelled
// emission (under-threshold, unflagged) carries the head marker.
func TestDetectingAggregator_UpstreamFlagPropagation_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 100).Draw(t, "lineLimit")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			1, lineLimit/2,
		).Draw(t, "raw")
		raw2 := rapid.SliceOfN(
			rapid.SampledFrom([]byte("uvwxy67890")),
			1, lineLimit/2,
		).Draw(t, "raw2")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		emittedA := captureDetectingEmissions(ag.Process(makeDetectingMessage(raw, true), aggregate, nil))
		emittedB := captureDetectingEmissions(ag.Process(makeDetectingMessage(raw2, false), aggregate, nil))
		if len(emittedA) != 1 || len(emittedB) != 1 {
			t.Fatalf("expected 1 emission each, got %d / %d", len(emittedA), len(emittedB))
		}

		marker := message.TruncatedFlag

		// Emission 1: under-threshold but upstream-flagged → tail
		// marker, IsTruncated=true.
		if !bytes.HasSuffix(emittedA[0].content, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: no tail marker on emission 1 (upstream flag) %q", emittedA[0].content)
		}
		if !emittedA[0].isTruncated {
			t.Fatal("UpstreamFlagPropagation violated: emission 1 IsTruncated=false despite upstream flag")
		}

		// Emission 2: under-threshold, unflagged, but carry was
		// set by emission 1 → head marker.
		if !bytes.HasPrefix(emittedB[0].content, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: emission 2 missing head marker after upstream-flagged carry; content %q", emittedB[0].content)
		}
	})
}

// TestDetectingAggregator_TagOnTruncation_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee TagOnTruncation — (inherited from Truncatable)
//	                                  when tagTruncatedLogs=true,
//	                                  a truncation-reason tag is
//	                                  added iff IsTruncated is set
//	                                  on the emission.
//	    @guarantee TagReasonSingleLine — tag value is "single_line".
//
// With tagTruncatedLogs=true at construction, every aggregate-
// labelled emission whose IsTruncated is true carries the
// truncation-reason tag "single_line" in its ParsingExtra.Tags.
// Emissions with IsTruncated=false do not carry it.
//
// Note: DetectingAggregator reads tagTruncatedLogs from its
// constructor parameter, NOT from the global config — distinct
// from PassThroughAggregator and SingleLineHandler.
func TestDetectingAggregator_TagOnTruncation_Property(t *testing.T) {
	expectedTag := message.TruncatedReasonTag("single_line")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genDetectingCall(), 1, 10).Draw(t, "calls")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, true, false)
		for i, c := range calls {
			// Force aggregate label so the message is emitted
			// immediately rather than potentially buffered.
			emitted := ag.Process(makeDetectingMessage([]byte(c.content), false), aggregate, c.tokens)
			for j, em := range emitted {
				e := em.Msg
				if e.ParsingExtra.IsTruncated {
					hasTag := false
					for _, tag := range e.ParsingExtra.Tags {
						if tag == expectedTag {
							hasTag = true
							break
						}
					}
					if !hasTag {
						t.Fatalf("TagOnTruncation/TagReasonSingleLine violated at call %d emission %d: IsTruncated=true but no %q in tags=%v", i, j, expectedTag, e.ParsingExtra.Tags)
					}
				} else {
					for _, tag := range e.ParsingExtra.Tags {
						if tag == expectedTag {
							t.Fatalf("TagOnTruncation violated at call %d emission %d: IsTruncated=false but tags contain %q (tags=%v)", i, j, expectedTag, e.ParsingExtra.Tags)
						}
					}
				}
			}
		}
	})
}

// TestDetectingAggregator_TagDisabledNoTag_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee TagOnTruncation — (inherited from Truncatable)
//	                                  when tagTruncatedLogs=false,
//	                                  NO truncation-reason tag is
//	                                  added regardless of
//	                                  IsTruncated.
//
// With tagTruncatedLogs=false at construction, no emission carries
// the "single_line" truncation-reason tag, even when markers are
// applied and IsTruncated is set.
func TestDetectingAggregator_TagDisabledNoTag_Property(t *testing.T) {
	suppressedTag := message.TruncatedReasonTag("single_line")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genDetectingCall(), 1, 10).Draw(t, "calls")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)
		for i, c := range calls {
			emitted := ag.Process(makeDetectingMessage([]byte(c.content), false), aggregate, c.tokens)
			for j, em := range emitted {
				for _, tag := range em.Msg.ParsingExtra.Tags {
					if tag == suppressedTag {
						t.Fatalf("TagOnTruncation (disabled) violated at call %d emission %d: found %q in tags=%v", i, j, suppressedTag, em.Msg.ParsingExtra.Tags)
					}
				}
			}
		}
	})
}

// TestDetectingAggregator_StartGroupBufferedUntilNextCall_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee StartGroupBufferedUntilNextCall — a process
//	                                                  call with the
//	                                                  start_group
//	                                                  label flushes
//	                                                  any pending
//	                                                  message then
//	                                                  buffers the
//	                                                  current
//	                                                  message; the
//	                                                  start_group
//	                                                  is NOT
//	                                                  emitted on
//	                                                  the call that
//	                                                  received it.
//
// On a fresh aggregator, a startGroup call produces zero
// emissions (the message is buffered as pending). The next call
// (aggregate, noAggregate, or another startGroup) releases the
// pending. A subsequent flush releases any final pending.
func TestDetectingAggregator_StartGroupBufferedUntilNextCall_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			1, 20,
		).Draw(t, "content"))

		ag := NewDetectingAggregator(status.NewInfoRegistry(), 200, false, false)
		emitted := ag.Process(makeDetectingMessage([]byte(content), false), startGroup, nil)
		if len(emitted) != 0 {
			t.Fatalf("StartGroupBufferedUntilNextCall violated: startGroup on fresh aggregator emitted %d messages, expected 0", len(emitted))
		}

		// Now release via flush.
		flushed := ag.Flush()
		if len(flushed) != 1 {
			t.Fatalf("StartGroupBufferedUntilNextCall violated: expected flush to release the pending startGroup, got %d emissions", len(flushed))
		}
	})
}

// TestDetectingAggregator_MultiLineDetectionTag_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee MultiLineDetectionTag — when a process call's
//	                                        label is aggregate AND
//	                                        a previous process call
//	                                        buffered a message with
//	                                        the start_group label,
//	                                        the buffered message is
//	                                        emitted with an
//	                                        additional tag
//	                                        "auto_multiline_detected:true".
//
// LOAD-BEARING for the refactor safety net. The detection tag is
// the WHOLE PURPOSE of DetectingAggregator; if the refactor
// changed when the tag is applied (e.g., applied unconditionally
// or skipped), behaviour would silently drift.
//
// Sequence: startGroup buffers msg A. aggregate emits A (with
// detection tag) and also emits B (without detection tag).
func TestDetectingAggregator_MultiLineDetectionTag_Property(t *testing.T) {
	detectionTag := "auto_multiline_detected:true"

	rapid.Check(t, func(t *rapid.T) {
		contentA := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef")),
			1, 10,
		).Draw(t, "contentA"))
		contentB := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("ghijkl")),
			1, 10,
		).Draw(t, "contentB"))

		ag := NewDetectingAggregator(status.NewInfoRegistry(), 200, false, false)
		ag.Process(makeDetectingMessage([]byte(contentA), false), startGroup, nil)
		emitted := captureDetectingEmissions(ag.Process(makeDetectingMessage([]byte(contentB), false), aggregate, nil))

		if len(emitted) != 2 {
			t.Fatalf("MultiLineDetectionTag setup: expected 2 emissions (pending + current), got %d", len(emitted))
		}
		// Emission 1: the pending startGroup, SHOULD have the
		// detection tag.
		hasTagOnFirst := false
		for _, tag := range emitted[0].tags {
			if tag == detectionTag {
				hasTagOnFirst = true
				break
			}
		}
		if !hasTagOnFirst {
			t.Fatalf("MultiLineDetectionTag violated: pending startGroup emission missing %q tag; tags=%v", detectionTag, emitted[0].tags)
		}
		// Emission 2: the current aggregate, should NOT have
		// the detection tag.
		for _, tag := range emitted[1].tags {
			if tag == detectionTag {
				t.Fatalf("MultiLineDetectionTag violated: current aggregate emission has unexpected %q tag (tag belongs only to detected start_group emissions); tags=%v", detectionTag, emitted[1].tags)
			}
		}
	})
}

// TestDetectingAggregator_PerEmissionTruncationFlow_Property anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PerEmissionTruncationFlow — within a single
//	                                            process call, when
//	                                            multiple emissions
//	                                            are produced, the
//	                                            Truncatable apply
//	                                            runs independently
//	                                            for each, with the
//	                                            carry flowing
//	                                            sequentially.
//
// A startGroup call followed by an aggregate call produces TWO
// emissions on the second process call: the pending startGroup
// message and the current aggregate message. When the startGroup
// content was over threshold, its emission carries the tail
// marker AND the aggregate emission (which immediately follows in
// the same call) carries the head marker — proving the carry
// flows sequentially across emissions within one process call.
func TestDetectingAggregator_PerEmissionTruncationFlow_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		startGroupContent := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "startGroupContent")
		aggregateContent := rapid.SliceOfN(
			rapid.SampledFrom([]byte("xyzwv98765")),
			1, lineLimit/2+1,
		).Draw(t, "aggregateContent")

		ag := NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, false, false)

		// Call 1: startGroup with over-threshold content. The
		// message is buffered; no emission yet.
		emitted1 := ag.Process(makeDetectingMessage(startGroupContent, false), startGroup, nil)
		if len(emitted1) != 0 {
			t.Fatalf("PerEmissionTruncationFlow precondition: call 1 (startGroup) should buffer and emit nothing, got %d emissions", len(emitted1))
		}

		// Call 2: aggregate with under-threshold content. Should
		// emit pending (over-threshold → tail marker, carry set)
		// then current (under-threshold + carry → head marker).
		emitted2 := ag.Process(makeDetectingMessage(aggregateContent, false), aggregate, nil)
		if len(emitted2) != 2 {
			t.Fatalf("PerEmissionTruncationFlow violated: expected 2 emissions on call 2 (aggregate-with-pending), got %d", len(emitted2))
		}

		marker := message.TruncatedFlag

		// Emission 1 (pending startGroup): tail marker (was over-
		// threshold), IsTruncated=true.
		e1 := emitted2[0].Msg
		if !bytes.HasSuffix(e1.GetContent(), marker) {
			t.Fatalf("PerEmissionTruncationFlow violated: emission 1 (pending over-threshold) missing tail marker; content %q", e1.GetContent())
		}
		if !e1.ParsingExtra.IsTruncated {
			t.Fatal("PerEmissionTruncationFlow violated: emission 1 IsTruncated=false despite over-threshold content")
		}

		// Emission 2 (current aggregate): head marker (from
		// emission 1's carry), IsTruncated=true.
		e2 := emitted2[1].Msg
		if !bytes.HasPrefix(e2.GetContent(), marker) {
			t.Fatalf("PerEmissionTruncationFlow violated: emission 2 missing head marker — the carry from emission 1 within the same process call must propagate; content %q", e2.GetContent())
		}
		if bytes.HasSuffix(e2.GetContent(), marker) {
			t.Fatalf("PerEmissionTruncationFlow violated: emission 2 has unexpected tail marker on under-threshold content; content %q", e2.GetContent())
		}
		if !e2.ParsingExtra.IsTruncated {
			t.Fatal("PerEmissionTruncationFlow violated: emission 2 IsTruncated=false on emission with head marker")
		}
	})
}
