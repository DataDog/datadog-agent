// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"pgregory.net/rapid"
)

// Property tests for the AutoMultilineLabeling surface declared in
// auto_multiline_labeler.allium. Each test names the spec @guarantee
// it anchors so that drift in either direction is easy to spot during
// review.
//
// Anchoring unit tests for the same surface live in
// auto_multiline_labeler_test.go.

// scriptedHeuristic generates a single mock Heuristic whose behaviour
// is fixed for the duration of the test (deterministic given the
// random draw). Each generated heuristic chooses, at draw time:
//
//   - whether to set context.label / context.labelAssignedBy
//   - which Label value to set (if any)
//   - which assigner_id string to record (if any)
//   - whether to return true (continue) or false (terminate)
//
// The generator covers all Label values from the enum, ensuring
// generated chains can drive the labeller to every possible output
// state. Heuristics that do NOT set the label do NOT modify
// labelAssignedBy, mirroring the LabelAssignedByConsistency invariant
// from the Heuristic contract.
func scriptedHeuristic() *rapid.Generator[Heuristic] {
	return rapid.Custom(func(t *rapid.T) Heuristic {
		setLabel := rapid.Bool().Draw(t, "setLabel")
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		assigner := rapid.SampledFrom([]string{"h_a", "h_b", "h_c"}).Draw(t, "assigner")
		shouldContinue := rapid.Bool().Draw(t, "continue")
		return &mockHeuristic{
			processFunc: func(ctx *messageContext) bool {
				if setLabel {
					ctx.label = label
					ctx.labelAssignedBy = assigner
				}
				return shouldContinue
			},
		}
	})
}

// labelInput bundles a randomly-generated (content, tokens,
// token_indices) triple. token_indices is generated aligned with
// tokens (length matches), per the Labeler contract's
// IndicesAlignment invariant — the labeller's behaviour is only
// defined when the caller honours alignment, so the generator
// produces only well-formed inputs.
type labelInput struct {
	content       []byte
	tokens        []Token
	tokenIndicies []int
}

func genLabelInput() *rapid.Generator[labelInput] {
	return rapid.Custom(func(t *rapid.T) labelInput {
		content := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "content")
		n := rapid.IntRange(0, 12).Draw(t, "nTokens")
		tokens := make([]Token, n)
		indicies := make([]int, n)
		for i := 0; i < n; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
			indicies[i] = rapid.IntRange(0, 200).Draw(t, "tokenIndex")
		}
		return labelInput{content: content, tokens: tokens, tokenIndicies: indicies}
	})
}

// TestAutoMultilineLabeler_Determinism_Property anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guarantee Determinism
//
// label() is a pure function of its arguments: the same
// (content, tokens, token_indices) tuple always produces the same
// Label value. For arbitrary chains of deterministic mock
// heuristics and arbitrary inputs, two back-to-back invocations of
// label() return the same value.
//
// The @guarantee's stated argument decomposes determinism into
// three sources: (a) HeuristicContext initialisation is purely a
// function of inputs, (b) each Heuristic is deterministic per its
// own contract, (c) iteration order is fixed. This test exercises
// (a) and (c) directly; the mocks model (b) by construction.
func TestAutoMultilineLabeler_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		labelling := rapid.SliceOfN(scriptedHeuristic(), 0, 6).Draw(t, "labelling")
		analytics := rapid.SliceOfN(scriptedHeuristic(), 0, 4).Draw(t, "analytics")
		input := genLabelInput().Draw(t, "input")

		labeler := NewLabeler(labelling, analytics)
		first := labeler.Label(input.content, input.tokens, input.tokenIndicies)
		second := labeler.Label(input.content, input.tokens, input.tokenIndicies)
		if first != second {
			t.Fatalf("Determinism violated: first=%v second=%v", first, second)
		}
	})
}

// TestAutoMultilineLabeler_Totality_Property anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guarantee Totality
//
// label() always returns one of the three Label enum values
// {start_group, no_aggregate, aggregate}. With mock heuristics
// that only emit valid Label values — honouring
// labeler/Heuristic.LabelDomain — the labeller never returns an
// out-of-enum value, regardless of chain length, input, or how
// the labelling chain terminated.
//
// The @guarantee's stated argument decomposes totality into three
// sources: (a) context.label initialised to aggregate (valid
// enum), (b) every Heuristic leaves context.label as an enum
// value, (c) iteration over the chains terminates. This test
// exercises (a) and (c) directly; the mocks model (b) by
// construction (scriptedHeuristic only samples from the enum).
func TestAutoMultilineLabeler_Totality_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		labelling := rapid.SliceOfN(scriptedHeuristic(), 0, 6).Draw(t, "labelling")
		analytics := rapid.SliceOfN(scriptedHeuristic(), 0, 4).Draw(t, "analytics")
		input := genLabelInput().Draw(t, "input")

		labeler := NewLabeler(labelling, analytics)
		result := labeler.Label(input.content, input.tokens, input.tokenIndicies)
		switch result {
		case startGroup, noAggregate, aggregate:
			// ok — value is in the Label enum
		default:
			t.Fatalf("Totality violated: returned label %v is not in {start_group, no_aggregate, aggregate}", result)
		}
	})
}
