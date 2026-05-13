// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"pgregory.net/rapid"

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
