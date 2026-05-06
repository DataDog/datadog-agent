// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"strings"
	"testing"

	"pgregory.net/rapid"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Property tests for the PassThroughAggregation surface declared in
// pass_through_aggregator.allium. Each test names the spec
// @guarantee it anchors so that drift in either direction is easy
// to spot during review.
//
// Anchoring unit tests for the same surface live in
// pass_through_aggregator_test.go.

// passThroughInput bundles a single randomly-generated process call.
type passThroughInput struct {
	content             []byte
	upstreamIsTruncated bool
	label               Label
	tokens              []Token
}

// genPassThroughInput produces a single call's worth of input. The
// generated content spans both sides of any reasonable line_limit so
// generators that draw a sequence of calls exercise both the
// "fits" and "oversize" branches of the @guidance step 2 truncation
// recomputation.
func genPassThroughInput() *rapid.Generator[passThroughInput] {
	return rapid.Custom(func(t *rapid.T) passThroughInput {
		// Bytes drawn from a printable range so the trim-space
		// effect is visually predictable. Spaces and tabs included
		// so trimming exercises both leading and trailing cases.
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef 0123\t")),
			0, 60,
		).Draw(t, "raw")
		isTruncated := rapid.Bool().Draw(t, "isTruncated")
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		nTokens := rapid.IntRange(0, 6).Draw(t, "nTokens")
		tokens := make([]Token, nTokens)
		for i := 0; i < nTokens; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
		}
		return passThroughInput{
			content:             raw,
			upstreamIsTruncated: isTruncated,
			label:               label,
			tokens:              tokens,
		}
	})
}

// runOne builds a fresh message from the generated input and calls
// process once. Returned values are copied so subsequent calls to
// process (which reuse the internal buffer) do not invalidate them
// — this is part of how the test honours the ResultLifetime
// invariant.
func runOne(ag *PassThroughAggregator, in passThroughInput) (content []byte, isTruncated bool, tokens []Token) {
	msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(in.content)
	msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
	emitted := ag.Process(msg, in.label, in.tokens)
	if len(emitted) != 1 {
		return nil, false, nil
	}
	content = append([]byte(nil), emitted[0].Msg.GetContent()...)
	isTruncated = emitted[0].Msg.ParsingExtra.IsTruncated
	tokens = append([]Token(nil), emitted[0].Tokens...)
	return content, isTruncated, tokens
}

// TestPassThroughAggregator_NoGrouping_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee NoGrouping — Every call to process emits exactly
//	                             one AggregatedMessageWithTokens
//
// Across a randomly-generated sequence of process calls of
// arbitrary length, every call emits exactly one
// AggregatedMessageWithTokens. PassThrough never buffers a line,
// regardless of label, content, or truncation state.
func TestPassThroughAggregator_NoGrouping_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ag := NewPassThroughAggregator(rapid.IntRange(1, 200).Draw(t, "lineLimit"))
		calls := rapid.SliceOfN(genPassThroughInput(), 1, 10).Draw(t, "calls")
		for i, in := range calls {
			msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
			msg.RawDataLen = len(in.content)
			msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
			emitted := ag.Process(msg, in.label, in.tokens)
			if len(emitted) != 1 {
				t.Fatalf("NoGrouping violated: call %d emitted %d messages, expected exactly 1", i, len(emitted))
			}
		}
	})
}

// TestPassThroughAggregator_IsEmptyAlwaysTrue_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee IsEmptyConsistency — is_empty always returns
//	                                     true (PassThrough has no
//	                                     buffer state)
//
// is_empty returns true after construction and after every process
// call, across arbitrary call sequences. The rolling
// should_truncate flag does not influence is_empty.
func TestPassThroughAggregator_IsEmptyAlwaysTrue_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ag := NewPassThroughAggregator(rapid.IntRange(1, 200).Draw(t, "lineLimit"))
		if !ag.IsEmpty() {
			t.Fatal("IsEmptyConsistency violated: is_empty false at construction")
		}
		calls := rapid.SliceOfN(genPassThroughInput(), 0, 10).Draw(t, "calls")
		for i, in := range calls {
			runOne(ag, in)
			if !ag.IsEmpty() {
				t.Fatalf("IsEmptyConsistency violated: is_empty false after call %d", i)
			}
		}
	})
}

// TestPassThroughAggregator_FlushEmpty_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — flush always returns an
//	                                    empty sequence (PassThrough
//	                                    has no buffer state)
//
// Flush returns an empty sequence after arbitrary process call
// sequences. The rolling should_truncate flag is not drained by
// flush.
func TestPassThroughAggregator_FlushEmpty_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ag := NewPassThroughAggregator(rapid.IntRange(1, 200).Draw(t, "lineLimit"))
		calls := rapid.SliceOfN(genPassThroughInput(), 0, 10).Draw(t, "calls")
		for _, in := range calls {
			runOne(ag, in)
		}
		if got := ag.Flush(); len(got) != 0 {
			t.Fatalf("FlushDrainsBuffer violated: flush returned %d messages, expected empty", len(got))
		}
	})
}

// TestPassThroughAggregator_LabelIgnored_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee LabelIgnored — The label argument is consumed
//	                               but has no observable effect on
//	                               the output sequence or on the
//	                               should_truncate state.
//
// For arbitrary content and truncation state, two aggregators
// fed an identical sequence of inputs (one with all-start_group
// labels, one with all-no_aggregate labels) produce identical
// emitted content, IsTruncated flags, and tokens. The label has
// no observable effect.
func TestPassThroughAggregator_LabelIgnored_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		agA := NewPassThroughAggregator(lineLimit)
		agB := NewPassThroughAggregator(lineLimit)
		calls := rapid.SliceOfN(genPassThroughInput(), 1, 8).Draw(t, "calls")
		for i, in := range calls {
			inA := in
			inA.label = startGroup
			inB := in
			inB.label = aggregate

			contentA, truncA, _ := runOne(agA, inA)
			contentB, truncB, _ := runOne(agB, inB)

			if !bytes.Equal(contentA, contentB) {
				t.Fatalf("LabelIgnored violated at call %d: content %q (start_group) vs %q (aggregate)", i, contentA, contentB)
			}
			if truncA != truncB {
				t.Fatalf("LabelIgnored violated at call %d: is_truncated %v (start_group) vs %v (aggregate)", i, truncA, truncB)
			}
		}
	})
}

// TestPassThroughAggregator_TokensForwarded_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee TokensForwarded — The tokens argument is
//	                                  forwarded unchanged into
//	                                  AggregatedMessageWithTokens.tokens
//
// For arbitrary inputs, the tokens emitted on the single output
// message are byte-equal to the tokens passed in.
func TestPassThroughAggregator_TokensForwarded_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ag := NewPassThroughAggregator(rapid.IntRange(1, 200).Draw(t, "lineLimit"))
		in := genPassThroughInput().Draw(t, "input")
		msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
		msg.RawDataLen = len(in.content)
		msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
		emitted := ag.Process(msg, in.label, in.tokens)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		if len(in.tokens) != len(emitted[0].Tokens) {
			t.Fatalf("TokensForwarded violated: length %d → %d", len(in.tokens), len(emitted[0].Tokens))
		}
		for i := range in.tokens {
			if in.tokens[i] != emitted[0].Tokens[i] {
				t.Fatalf("TokensForwarded violated: tokens[%d] %v → %v", i, in.tokens[i], emitted[0].Tokens[i])
			}
		}
	})
}

// TestPassThroughAggregator_ByteConservation_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee ByteConservation — emitted bytes derive from
//	                                   trimmed input plus optional
//	                                   prepended/appended truncation
//	                                   markers
//
// The emitted content, after stripping any prepended/appended
// truncation marker, must equal the trim-spaced input bytes. This
// is the strong form: every byte in the output traces back to
// either (a) a non-whitespace input byte, or (b) the truncation
// marker. No other bytes appear.
func TestPassThroughAggregator_ByteConservation_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ag := NewPassThroughAggregator(rapid.IntRange(1, 200).Draw(t, "lineLimit"))
		in := genPassThroughInput().Draw(t, "input")
		msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
		msg.RawDataLen = len(in.content)
		msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
		emitted := ag.Process(msg, in.label, in.tokens)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}

		out := string(emitted[0].Msg.GetContent())
		marker := string(message.TruncatedFlag)

		// Strip at most one prepended and one appended marker.
		stripped := out
		stripped = strings.TrimPrefix(stripped, marker)
		stripped = strings.TrimSuffix(stripped, marker)

		expectedCore := strings.TrimSpace(string(in.content))
		if stripped != expectedCore {
			t.Fatalf("ByteConservation violated: stripped %q != trimmed input %q (full output %q)",
				stripped, expectedCore, out)
		}
	})
}

// TestPassThroughAggregator_PassThroughUnderThreshold_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee PassThroughUnderThreshold — (inherited from
//	                                            Truncatable) under-
//	                                            threshold + no carry
//	                                            + no upstream flag ⇒
//	                                            no markers, IsTruncated
//	                                            false.
//
// A single process call on a fresh aggregator whose content is
// strictly under line_limit and whose ParsingExtra.IsTruncated
// is false emits trim-spaced content with no truncation markers
// and IsTruncated=false.
func TestPassThroughAggregator_PassThroughUnderThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(50, 200).Draw(t, "lineLimit")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef 0123\t")),
			0, lineLimit/2,
		).Draw(t, "raw")
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")

		ag := NewPassThroughAggregator(lineLimit)
		in := passThroughInput{content: raw, upstreamIsTruncated: false, label: label}
		content, isTruncated, _ := runOne(ag, in)

		marker := message.TruncatedFlag
		if bytes.HasPrefix(content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: head marker present in %q", content)
		}
		if bytes.HasSuffix(content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: tail marker present in %q", content)
		}
		if isTruncated {
			t.Fatal("PassThroughUnderThreshold violated: IsTruncated=true on under-threshold non-upstream input")
		}
		expected := bytes.TrimSpace(raw)
		if !bytes.Equal(content, expected) {
			t.Fatalf("PassThroughUnderThreshold violated: content %q != trimmed input %q", content, expected)
		}
	})
}

// TestPassThroughAggregator_TailMarkerOnOverThreshold_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee TailMarkerOnOverThreshold — (inherited from
//	                                            Truncatable) content
//	                                            over threshold ⇒
//	                                            tail marker,
//	                                            IsTruncated true.
//
// A single process call on a fresh aggregator whose content has
// byte length strictly greater than line_limit emits content
// ending with the truncation marker and IsTruncated=true.
func TestPassThroughAggregator_TailMarkerOnOverThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "raw")
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")

		ag := NewPassThroughAggregator(lineLimit)
		in := passThroughInput{content: raw, upstreamIsTruncated: false, label: label}
		content, isTruncated, _ := runOne(ag, in)

		marker := message.TruncatedFlag
		if !bytes.HasSuffix(content, marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: no tail marker in %q", content)
		}
		if bytes.HasPrefix(content, marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: unexpected head marker on fresh aggregator in %q", content)
		}
		if !isTruncated {
			t.Fatal("TailMarkerOnOverThreshold violated: IsTruncated=false on over-threshold input")
		}
	})
}

// TestPassThroughAggregator_HeadMarkerOnCarryover_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee HeadMarkerOnCarryover — (inherited from
//	                                        Truncatable) prior carry
//	                                        ⇒ head marker on next
//	                                        emission, IsTruncated
//	                                        true.
//
// After a process call that truncates (setting carry=true), the
// NEXT process call's emission begins with the truncation marker
// and IsTruncated=true, even when its own content is under
// threshold and not upstream-flagged.
func TestPassThroughAggregator_HeadMarkerOnCarryover_Property(t *testing.T) {
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
		labelA := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelA")
		labelB := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelB")

		ag := NewPassThroughAggregator(lineLimit)
		runOne(ag, passThroughInput{content: first, upstreamIsTruncated: false, label: labelA})
		content, isTruncated, _ := runOne(ag, passThroughInput{content: second, upstreamIsTruncated: false, label: labelB})

		marker := message.TruncatedFlag
		if !bytes.HasPrefix(content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on second emission %q", content)
		}
		if bytes.HasSuffix(content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: unexpected tail marker on under-threshold second emission %q", content)
		}
		if !isTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestPassThroughAggregator_CarryoverConsumed_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee CarryoverConsumed — (inherited from Truncatable)
//	                                    carry is determined solely
//	                                    by current input; prior
//	                                    carry does not propagate.
//
// After a process call that sets carry (over-threshold input),
// a next call under threshold and not upstream-flagged consumes
// the carry (emitting with head marker) and the THIRD call must
// NOT carry the marker — verifying carry reset on emission 2.
func TestPassThroughAggregator_CarryoverConsumed_Property(t *testing.T) {
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
		labelA := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelA")
		labelB := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelB")
		labelC := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelC")

		ag := NewPassThroughAggregator(lineLimit)
		runOne(ag, passThroughInput{content: first, upstreamIsTruncated: false, label: labelA})
		contentB, _, _ := runOne(ag, passThroughInput{content: second, upstreamIsTruncated: false, label: labelB})
		contentC, isTruncatedC, _ := runOne(ag, passThroughInput{content: third, upstreamIsTruncated: false, label: labelC})

		marker := message.TruncatedFlag

		// Precondition: emission 2 should carry the head marker.
		if !bytes.HasPrefix(contentB, marker) {
			t.Fatalf("CarryoverConsumed precondition: emission 2 should have head marker; got %q", contentB)
		}

		// The point: emission 3 must NOT carry a head marker.
		if bytes.HasPrefix(contentC, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 carries head marker after emission 2 should have consumed it; content %q", contentC)
		}
		if bytes.HasSuffix(contentC, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 has tail marker on under-threshold input; content %q", contentC)
		}
		if isTruncatedC {
			t.Fatal("CarryoverConsumed violated: emission 3 IsTruncated=true on clean under-threshold input")
		}
	})
}

// TestPassThroughAggregator_UpstreamFlagPropagation_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee UpstreamFlagPropagation — (inherited from
//	                                          Truncatable) upstream
//	                                          IsTruncated true
//	                                          behaves like over-
//	                                          threshold: tail marker,
//	                                          carry set, IsTruncated
//	                                          true.
//
// On a fresh aggregator, an under-threshold input whose
// ParsingExtra.IsTruncated=true emits with the tail marker and
// IsTruncated=true. The next emission (under-threshold, no flag)
// carries the head marker (carry was set by emission 1).
func TestPassThroughAggregator_UpstreamFlagPropagation_Property(t *testing.T) {
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
		labelA := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelA")
		labelB := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "labelB")

		ag := NewPassThroughAggregator(lineLimit)
		contentA, isTruncatedA, _ := runOne(ag, passThroughInput{content: raw, upstreamIsTruncated: true, label: labelA})
		contentB, _, _ := runOne(ag, passThroughInput{content: raw2, upstreamIsTruncated: false, label: labelB})

		marker := message.TruncatedFlag

		// Emission 1: under-threshold but upstream-flagged → tail
		// marker, IsTruncated=true.
		if !bytes.HasSuffix(contentA, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: no tail marker on emission 1 (upstream flag) %q", contentA)
		}
		if !isTruncatedA {
			t.Fatal("UpstreamFlagPropagation violated: emission 1 IsTruncated=false despite upstream flag")
		}

		// Emission 2: under-threshold, unflagged, but carry was
		// set by emission 1 → head marker.
		if !bytes.HasPrefix(contentB, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: emission 2 missing head marker after upstream-flagged carry; content %q", contentB)
		}
	})
}

// TestPassThroughAggregator_TagOnTruncation_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee TagOnTruncation — (inherited from Truncatable)
//	                                  when global
//	                                  logs_config.tag_truncated_logs
//	                                  is true, a truncation-reason
//	                                  tag is added iff IsTruncated
//	                                  is set on the emission.
//	    @guarantee TagReasonSingleLine — tag value is "single_line".
//
// With the global config enabled, every emission whose
// IsTruncated is true carries exactly one ParsingExtra.Tags
// entry equal to message.TruncatedReasonTag("single_line").
// Emissions with IsTruncated=false carry no tag.
func TestPassThroughAggregator_TagOnTruncation_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", true)
	expectedTag := message.TruncatedReasonTag("single_line")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genPassThroughInput(), 1, 10).Draw(t, "calls")

		ag := NewPassThroughAggregator(lineLimit)
		for i, in := range calls {
			msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
			msg.RawDataLen = len(in.content)
			msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
			emitted := ag.Process(msg, in.label, in.tokens)
			if len(emitted) != 1 {
				t.Fatalf("call %d: expected 1 emission, got %d", i, len(emitted))
			}
			e := emitted[0].Msg
			if e.ParsingExtra.IsTruncated {
				if len(e.ParsingExtra.Tags) != 1 || e.ParsingExtra.Tags[0] != expectedTag {
					t.Fatalf("TagOnTruncation/TagReasonSingleLine violated at call %d: IsTruncated=true but tags=%v (want [%q])", i, e.ParsingExtra.Tags, expectedTag)
				}
			} else {
				if len(e.ParsingExtra.Tags) != 0 {
					t.Fatalf("TagOnTruncation violated at call %d: IsTruncated=false but tags=%v (want none)", i, e.ParsingExtra.Tags)
				}
			}
		}
	})
}

// TestPassThroughAggregator_TagDisabledNoTag_Property anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee TagOnTruncation — (inherited from Truncatable)
//	                                  when global
//	                                  logs_config.tag_truncated_logs
//	                                  is false, NO truncation-reason
//	                                  tag is added regardless of
//	                                  IsTruncated.
//
// With the global config disabled, no emission carries a
// truncation-reason tag, even when markers are applied and
// IsTruncated is set.
func TestPassThroughAggregator_TagDisabledNoTag_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", false)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genPassThroughInput(), 1, 10).Draw(t, "calls")

		ag := NewPassThroughAggregator(lineLimit)
		for i, in := range calls {
			msg := message.NewMessage(append([]byte(nil), in.content...), nil, message.StatusInfo, 0)
			msg.RawDataLen = len(in.content)
			msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
			emitted := ag.Process(msg, in.label, in.tokens)
			if len(emitted) != 1 {
				t.Fatalf("call %d: expected 1 emission, got %d", i, len(emitted))
			}
			if len(emitted[0].Msg.ParsingExtra.Tags) != 0 {
				t.Fatalf("TagOnTruncation (disabled) violated at call %d: tags=%v (want none)", i, emitted[0].Msg.ParsingExtra.Tags)
			}
		}
	})
}
