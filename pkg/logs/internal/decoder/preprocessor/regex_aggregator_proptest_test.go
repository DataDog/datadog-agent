// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"regexp"
	"testing"

	"pgregory.net/rapid"

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
func TestRegexAggregator_FlushEqualsTotalEmissions_Property(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genRegexAggregatorCall(), 1, 10).Draw(t, "calls")
		// Force the final line to be non-matching content so the
		// buffer is non-empty when Flush is called.
		calls[len(calls)-1].content = "trailing non-match"
		ag := NewRegexAggregator(re, 100, false, status.NewInfoRegistry(), "multi_line")
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
