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
