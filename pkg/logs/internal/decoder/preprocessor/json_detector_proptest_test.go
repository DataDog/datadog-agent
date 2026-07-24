// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"
)

// Property tests for the JSONDetection surface declared in
// json_detector.allium. Each test names the spec @guarantee it
// anchors so that drift in either direction is easy to spot during
// review.
//
// Anchoring unit tests for the same surface live in
// json_detector_test.go.

// genDetectorInput bundles a randomly-generated context state for
// JSONDetector. The fields cover every input the detector can
// observe: raw_message bytes, a tokens / token_indices pair (the
// detector should ignore these per InputImmutability), and a
// label / label_assigned_by pair (the detector may observe these
// to decide deference). The fields are independent — the
// generator does not bias toward JSON-shaped raw messages, so
// claim and no-claim paths are both exercised.
type detectorInput struct {
	rawMessage      []byte
	tokens          []Token
	tokenIndicies   []int
	label           Label
	labelAssignedBy string
}

func genDetectorInput() *rapid.Generator[detectorInput] {
	return rapid.Custom(func(t *rapid.T) detectorInput {
		raw := rapid.SliceOfN(rapid.Byte(), 0, 80).Draw(t, "raw")
		n := rapid.IntRange(0, 12).Draw(t, "nTokens")
		tokens := make([]Token, n)
		indicies := make([]int, n)
		for i := 0; i < n; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
			indicies[i] = rapid.IntRange(0, 200).Draw(t, "tokenIndex")
		}
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		// SampledFrom covers both the "default" sentinel and various
		// prior-assigner ids, exercising deference (case 1) and
		// non-deference (cases 2 and 3) branches.
		assignedBy := rapid.SampledFrom([]string{defaultLabelSource, "JSON_detector", "timestamp_detector", "other"}).Draw(t, "assignedBy")
		return detectorInput{
			rawMessage:      raw,
			tokens:          tokens,
			tokenIndicies:   indicies,
			label:           label,
			labelAssignedBy: assignedBy,
		}
	})
}

// makeDetectorContext returns a fresh messageContext populated from the
// generated input. The returned context owns its own copies of the
// byte/Token/int slices so tests can compare pre-call and post-call
// state without aliasing.
func makeDetectorContext(in detectorInput) *messageContext {
	return &messageContext{
		rawMessage:      append([]byte(nil), in.rawMessage...),
		tokens:          append([]Token(nil), in.tokens...),
		tokenIndicies:   append([]int(nil), in.tokenIndicies...),
		label:           in.label,
		labelAssignedBy: in.labelAssignedBy,
	}
}

// TestJSONDetector_Determinism_Property anchors:
//
//	contract Heuristic (labeler.allium)
//	    @invariant TerminationSemantics — A Heuristic must return
//	                                       a stable value for a
//	                                       given context state
//
// (Determinism is the Heuristic contract's general requirement;
// JSONDetection's @guarantee TerminationSemantics inherits it.)
//
// Two back-to-back invocations of ProcessAndContinue on
// equivalent fresh contexts produce equal return values and equal
// post-call context state — both the label and label_assigned_by
// transitions are deterministic functions of the input.
func TestJSONDetector_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genDetectorInput().Draw(t, "input")
		d := NewJSONDetector()

		ctxA := makeDetectorContext(in)
		retA := d.ProcessAndContinue(ctxA)

		ctxB := makeDetectorContext(in)
		retB := d.ProcessAndContinue(ctxB)

		if retA != retB {
			t.Fatalf("Determinism violated (return value): %v vs %v", retA, retB)
		}
		if ctxA.label != ctxB.label {
			t.Fatalf("Determinism violated (label): %v vs %v", ctxA.label, ctxB.label)
		}
		if ctxA.labelAssignedBy != ctxB.labelAssignedBy {
			t.Fatalf("Determinism violated (labelAssignedBy): %q vs %q", ctxA.labelAssignedBy, ctxB.labelAssignedBy)
		}
	})
}

// TestJSONDetector_LabelDomain_Property anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee LabelDomain — when JSONDetector claims the
//	                              label, it sets context.label to
//	                              no_aggregate
//
// The strong form: whenever ProcessAndContinue returns false (the
// claim path, per @guarantee TerminationSemantics), context.label
// MUST be exactly no_aggregate. JSONDetector never claims with
// any other label value, regardless of input.
//
// Sufficient (but not strictly necessary): also pin that the
// returned label is always within the Label enum.
func TestJSONDetector_LabelDomain_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genDetectorInput().Draw(t, "input")
		ctx := makeDetectorContext(in)
		ret := NewJSONDetector().ProcessAndContinue(ctx)

		switch ctx.label {
		case startGroup, noAggregate, aggregate:
			// ok — value is in the Label enum
		default:
			t.Fatalf("post-call label %v is not in the Label enum", ctx.label)
		}

		// If the detector claimed, the label MUST be no_aggregate.
		if !ret && ctx.label != noAggregate {
			t.Fatalf("claim path produced label %v, expected no_aggregate", ctx.label)
		}
	})
}

// TestJSONDetector_LabelAssignedByConsistency_Property anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee LabelAssignedByConsistency
//
// The biconditional form of LabelAssignedByConsistency: across all
// inputs, EITHER both context.label and context.label_assigned_by
// change, OR neither does. The detector never mutates only one of
// the pair.
//
// Restating: changed(label) ⇔ changed(label_assigned_by).
func TestJSONDetector_LabelAssignedByConsistency_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genDetectorInput().Draw(t, "input")
		ctx := makeDetectorContext(in)
		labelBefore := ctx.label
		assignedByBefore := ctx.labelAssignedBy

		NewJSONDetector().ProcessAndContinue(ctx)

		labelChanged := ctx.label != labelBefore
		assignedByChanged := ctx.labelAssignedBy != assignedByBefore

		if labelChanged != assignedByChanged {
			t.Fatalf("LabelAssignedByConsistency violated: labelChanged=%v assignedByChanged=%v (label %v→%v, assignedBy %q→%q)",
				labelChanged, assignedByChanged,
				labelBefore, ctx.label,
				assignedByBefore, ctx.labelAssignedBy)
		}
	})
}

// TestJSONDetector_InputImmutability_Property anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee InputImmutability — JSONDetector reads
//	                                    context.raw_message but
//	                                    never modifies it. It does
//	                                    not read or modify
//	                                    context.tokens or
//	                                    context.token_indices.
//
// Across all inputs, the underlying byte/Token/int slices held by
// the context are bitwise-equal to their pre-call snapshots after
// ProcessAndContinue returns. This catches both in-place mutation
// and unexpected aliasing.
func TestJSONDetector_InputImmutability_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genDetectorInput().Draw(t, "input")
		ctx := makeDetectorContext(in)

		rawSnapshot := append([]byte(nil), ctx.rawMessage...)
		tokensSnapshot := append([]Token(nil), ctx.tokens...)
		indicesSnapshot := append([]int(nil), ctx.tokenIndicies...)

		NewJSONDetector().ProcessAndContinue(ctx)

		if !bytes.Equal(rawSnapshot, ctx.rawMessage) {
			t.Fatalf("InputImmutability violated: raw_message mutated\nbefore: %q\nafter:  %q", rawSnapshot, ctx.rawMessage)
		}
		if len(tokensSnapshot) != len(ctx.tokens) {
			t.Fatalf("InputImmutability violated: tokens length changed %d → %d", len(tokensSnapshot), len(ctx.tokens))
		}
		for i := range tokensSnapshot {
			if tokensSnapshot[i] != ctx.tokens[i] {
				t.Fatalf("InputImmutability violated: tokens[%d] %v → %v", i, tokensSnapshot[i], ctx.tokens[i])
			}
		}
		if len(indicesSnapshot) != len(ctx.tokenIndicies) {
			t.Fatalf("InputImmutability violated: token_indices length changed %d → %d", len(indicesSnapshot), len(ctx.tokenIndicies))
		}
		for i := range indicesSnapshot {
			if indicesSnapshot[i] != ctx.tokenIndicies[i] {
				t.Fatalf("InputImmutability violated: token_indices[%d] %v → %v", i, indicesSnapshot[i], ctx.tokenIndicies[i])
			}
		}
	})
}
