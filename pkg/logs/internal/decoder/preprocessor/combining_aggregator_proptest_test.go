// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the CombiningAggregation surface declared in
// combining_aggregator.allium. Each test names the spec
// @guarantee it anchors so that drift in either direction is easy
// to spot during review.
//
// Anchoring unit tests for the same surface live in
// combining_aggregator_test.go and aggregator_test.go.

type combiningCall struct {
	content string
	label   Label
	tokens  []Token
}

func genCombiningCall() *rapid.Generator[combiningCall] {
	return rapid.Custom(func(t *rapid.T) combiningCall {
		content := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abc 012")),
			0, 20,
		).Draw(t, "content"))
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		nTokens := rapid.IntRange(0, 4).Draw(t, "nTokens")
		tokens := make([]Token, nTokens)
		for i := 0; i < nTokens; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
		}
		return combiningCall{content: content, label: label, tokens: tokens}
	})
}

// combiningEmission captures one emitted message's observable state.
type combiningEmission struct {
	content     string
	isTruncated bool
	isMultiLine bool
	tagsLen     int
	tokensLen   int
}

func runCombiningCalls(lineLimit int, tagTrunc, tagMulti bool, calls []combiningCall) []combiningEmission {
	ag := NewCombiningAggregator(lineLimit, tagTrunc, tagMulti, status.NewInfoRegistry())
	var out []combiningEmission
	for _, c := range calls {
		emitted := ag.Process(newMessage(c.content), c.label, c.tokens)
		for _, e := range emitted {
			out = append(out, combiningEmission{
				content:     string(e.Msg.GetContent()),
				isTruncated: e.Msg.ParsingExtra.IsTruncated,
				isMultiLine: e.Msg.ParsingExtra.IsMultiLine,
				tagsLen:     len(e.Msg.ParsingExtra.Tags),
				tokensLen:   len(e.Tokens),
			})
		}
	}
	for _, e := range ag.Flush() {
		out = append(out, combiningEmission{
			content:     string(e.Msg.GetContent()),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			isMultiLine: e.Msg.ParsingExtra.IsMultiLine,
			tagsLen:     len(e.Msg.ParsingExtra.Tags),
			tokensLen:   len(e.Tokens),
		})
	}
	return out
}

// TestCombiningAggregator_Determinism_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee Determinism
//
// Two aggregators of identical configuration, fed identical call
// sequences, produce equal observed emission sequences.
func TestCombiningAggregator_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 1, 12).Draw(t, "calls")
		lineLimit := rapid.IntRange(1, 100).Draw(t, "lineLimit")

		outA := runCombiningCalls(lineLimit, false, false, calls)
		outB := runCombiningCalls(lineLimit, false, false, calls)

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

// TestCombiningAggregator_FlushClearsBucket_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — after flush, bucket_empty
//	                                    is true.
//	    @guarantee IsEmptyConsistency
//
// Across arbitrary call sequences, flush followed by is_empty
// always yields true. A second flush returns empty.
func TestCombiningAggregator_FlushClearsBucket_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 0, 12).Draw(t, "calls")
		ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())
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

// TestCombiningAggregator_NoCombinedOverflow_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee OverflowExplosion — CombiningAggregator never
//	                                    produces a combined
//	                                    emission whose body
//	                                    crosses line_limit purely
//	                                    because of multi-line
//	                                    aggregation.
//
// The strong form: across arbitrary call sequences with arbitrary
// line_limits, no emission's content (minus markers) exceeds
// line_limit when the emission is multi-line (is_multi_line set).
// A 2+ line combined emission staying under line_limit is the
// observable manifestation of OverflowExplosion's "abandon
// combination rather than truncate-combine" policy.
func TestCombiningAggregator_NoCombinedOverflow_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 1, 12).Draw(t, "calls")
		// line_limit must be big enough to fit at least small inputs,
		// otherwise everything goes through the oversize-single-line
		// paths and the multi-line case never fires. >= 20 covers
		// typical small generated content.
		lineLimit := rapid.IntRange(20, 200).Draw(t, "lineLimit")

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		marker := string(message.TruncatedFlag)
		check := func(emitted []AggregatedMessageWithTokens) {
			for _, e := range emitted {
				if !e.Msg.ParsingExtra.IsMultiLine {
					continue
				}
				body := strings.TrimPrefix(string(e.Msg.GetContent()), marker)
				body = strings.TrimSuffix(body, marker)
				if len(body) >= lineLimit {
					t.Fatalf("NoCombinedOverflow violated: multi-line emission body %d bytes >= line_limit %d (content %q)",
						len(body), lineLimit, e.Msg.GetContent())
				}
			}
		}
		for _, c := range calls {
			check(ag.Process(newMessage(c.content), c.label, c.tokens))
		}
		check(ag.Flush())
	})
}

// TestCombiningAggregator_NoAggregateClearsCarry_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateResetsCarry — no_aggregate resets
//	                                         should_truncate to
//	                                         false before
//	                                         processing.
//
// The observable manifestation: across arbitrary inputs, a
// no_aggregate-labelled line whose content fits within line_limit
// and is not upstream-flagged never emerges with a prepended
// truncation marker — even if the prior emission set the carry.
//
// We construct a deterministic preamble that sets the carry (an
// oversized startGroup), then sample a no_aggregate continuation.
// The no_aggregate line must not have the prepend marker.
func TestCombiningAggregator_NoAggregateClearsCarry_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 50).Draw(t, "lineLimit")
		// Construct fitting content for the no_aggregate line: must
		// be <= line_limit and not upstream-flagged.
		contentLen := rapid.IntRange(1, lineLimit).Draw(t, "contentLen")
		content := strings.Repeat("x", contentLen)
		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())

		// Set the carry: oversized startGroup. Forces an immediate
		// single-line truncated emission and leaves should_truncate
		// set to true.
		oversized := strings.Repeat("y", lineLimit*2)
		msg := newMessage(oversized)
		msg.RawDataLen = len(oversized)
		first := ag.Process(msg, startGroup, nil)
		if len(first) != 1 {
			return // skip degenerate cases where startGroup doesn't immediately flush
		}

		// no_aggregate next: should NOT receive the prepended marker.
		nextMsg := newMessage(content)
		nextMsg.RawDataLen = len(content)
		emitted := ag.Process(nextMsg, noAggregate, nil)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission for fitting no_aggregate, got %d", len(emitted))
		}
		emittedContent := string(emitted[0].Msg.GetContent())
		if strings.HasPrefix(emittedContent, string(message.TruncatedFlag)) {
			t.Fatalf("NoAggregateResetsCarry violated: no_aggregate emission has prepended marker (content=%q)", emittedContent)
		}
	})
}
