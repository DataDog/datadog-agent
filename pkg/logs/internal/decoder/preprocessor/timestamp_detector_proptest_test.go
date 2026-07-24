// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Property tests for the TimestampDetection surface declared in
// timestamp_detector.allium. Each test names the spec @guarantee or
// @guidance step it anchors so that drift in either direction is
// easy to spot during review.
//
// Anchoring unit tests for the same surface live in
// timestamp_detector_test.go.

// timestampDetectorInput bundles a randomly-generated context state
// for TimestampDetector. The fields cover every observable input:
// raw_message bytes (the detector should ignore these per
// InputImmutability), a tokens / token_indices pair (the detector
// reads tokens to score against the shape model), and a
// label / label_assigned_by pair (the detector does not consult
// label_assigned_by — there is no deference case — but the fields
// are present so we can verify LabelAssignedByConsistency).
type timestampDetectorInput struct {
	rawMessage      []byte
	tokens          []Token
	tokenIndicies   []int
	label           Label
	labelAssignedBy string
}

func genTimestampDetectorInput() *rapid.Generator[timestampDetectorInput] {
	return rapid.Custom(func(t *rapid.T) timestampDetectorInput {
		raw := rapid.SliceOfN(rapid.Byte(), 0, 80).Draw(t, "raw")
		// Token sequences of length up to ~20 — the detector's
		// minimumTokenLength is 8, so generating across that boundary
		// exercises both the "too short to evaluate" path and the
		// scoring path.
		n := rapid.IntRange(0, 20).Draw(t, "nTokens")
		tokens := make([]Token, n)
		indicies := make([]int, n)
		for i := 0; i < n; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
			indicies[i] = rapid.IntRange(0, 200).Draw(t, "tokenIndex")
		}
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		assignedBy := rapid.SampledFrom([]string{defaultLabelSource, "timestamp_detector", "JSON_detector", "other"}).Draw(t, "assignedBy")
		return timestampDetectorInput{
			rawMessage:      raw,
			tokens:          tokens,
			tokenIndicies:   indicies,
			label:           label,
			labelAssignedBy: assignedBy,
		}
	})
}

// makeTimestampContext returns a fresh messageContext populated from
// the generated input. The returned context owns its own copies of
// the byte/Token/int slices so tests can compare pre-call and
// post-call state without aliasing.
func makeTimestampContext(in timestampDetectorInput) *messageContext {
	return &messageContext{
		rawMessage:      append([]byte(nil), in.rawMessage...),
		tokens:          append([]Token(nil), in.tokens...),
		tokenIndicies:   append([]int(nil), in.tokenIndicies...),
		label:           in.label,
		labelAssignedBy: in.labelAssignedBy,
	}
}

// newTimestampDetectorAndTokenizerForProptests builds a detector and
// tokenizer using the config-default settings, matching production
// wiring. Called once per Test* function (outside rapid.Check) so
// the *testing.T it captures stays valid; the returned values are
// reused across all rapid iterations.
func newTimestampDetectorAndTokenizerForProptests(t *testing.T) (*TimestampDetector, *Tokenizer) {
	t.Helper()
	mockConfig := configmock.New(t)
	return NewTimestampDetector(mockConfig.GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold")),
		NewTokenizer(mockConfig.GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes"))
}

// TestTimestampDetector_Determinism_Property anchors:
//
//	contract Heuristic (labeler.allium)
//	    @invariant TerminationSemantics — A Heuristic must return
//	                                       a stable value for a
//	                                       given context state
//
// (Determinism is the Heuristic contract's general requirement;
// TimestampDetection's @guarantee TerminationSemantics inherits
// it.) Two back-to-back invocations of ProcessAndContinue on
// equivalent fresh contexts produce equal return values and equal
// post-call context state.
func TestTimestampDetector_Determinism_Property(t *testing.T) {
	d, _ := newTimestampDetectorAndTokenizerForProptests(t)
	rapid.Check(t, func(t *rapid.T) {
		in := genTimestampDetectorInput().Draw(t, "input")

		ctxA := makeTimestampContext(in)
		retA := d.ProcessAndContinue(ctxA)

		ctxB := makeTimestampContext(in)
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

// TestTimestampDetector_LabelDomain_Property anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee LabelDomain — when TimestampDetector claims the
//	                              label, it sets context.label to
//	                              start_group
//
// The strong form: across arbitrary inputs, the post-call label
// is in the Label enum, and if the detector changed the label
// (claim path), the new value MUST be start_group. Random inputs
// rarely hit the claim path, so this property test mostly
// exercises the no-claim branch — the directed
// KnownFormatsClaim test below provides claim-path coverage.
func TestTimestampDetector_LabelDomain_Property(t *testing.T) {
	d, _ := newTimestampDetectorAndTokenizerForProptests(t)
	rapid.Check(t, func(t *rapid.T) {
		in := genTimestampDetectorInput().Draw(t, "input")
		ctx := makeTimestampContext(in)
		labelBefore := ctx.label

		d.ProcessAndContinue(ctx)

		switch ctx.label {
		case startGroup, noAggregate, aggregate:
			// ok — value is in the Label enum
		default:
			t.Fatalf("post-call label %v is not in the Label enum", ctx.label)
		}

		if ctx.label != labelBefore && ctx.label != startGroup {
			t.Fatalf("claim path produced label %v, expected start_group", ctx.label)
		}
	})
}

// TestTimestampDetector_LabelAssignedByConsistency_Property anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee LabelAssignedByConsistency
//
// The biconditional form: across arbitrary inputs, EITHER both
// context.label and context.label_assigned_by change, OR neither
// does. TimestampDetector never mutates only one of the pair.
func TestTimestampDetector_LabelAssignedByConsistency_Property(t *testing.T) {
	d, _ := newTimestampDetectorAndTokenizerForProptests(t)
	rapid.Check(t, func(t *rapid.T) {
		in := genTimestampDetectorInput().Draw(t, "input")
		ctx := makeTimestampContext(in)
		labelBefore := ctx.label
		assignedByBefore := ctx.labelAssignedBy

		d.ProcessAndContinue(ctx)

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

// TestTimestampDetector_InputImmutability_Property anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee InputImmutability — TimestampDetector reads
//	                                    context.tokens but never
//	                                    modifies it. It does not
//	                                    read or modify
//	                                    context.raw_message or
//	                                    context.token_indices.
//
// Across arbitrary inputs, the underlying byte/Token/int slices
// held by the context are bitwise-equal to their pre-call
// snapshots after ProcessAndContinue returns.
func TestTimestampDetector_InputImmutability_Property(t *testing.T) {
	d, _ := newTimestampDetectorAndTokenizerForProptests(t)
	rapid.Check(t, func(t *rapid.T) {
		in := genTimestampDetectorInput().Draw(t, "input")
		ctx := makeTimestampContext(in)

		rawSnapshot := append([]byte(nil), ctx.rawMessage...)
		tokensSnapshot := append([]Token(nil), ctx.tokens...)
		indicesSnapshot := append([]int(nil), ctx.tokenIndicies...)

		d.ProcessAndContinue(ctx)

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

// TestTimestampDetector_TerminationSemantics_Property anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee TerminationSemantics — process_and_continue
//	                                       always returns true
//
// The advisory-heuristic property: the return value is true on
// every input, regardless of which @guidance case fires.
func TestTimestampDetector_TerminationSemantics_Property(t *testing.T) {
	d, _ := newTimestampDetectorAndTokenizerForProptests(t)
	rapid.Check(t, func(t *rapid.T) {
		in := genTimestampDetectorInput().Draw(t, "input")
		ctx := makeTimestampContext(in)

		ret := d.ProcessAndContinue(ctx)
		if !ret {
			t.Fatalf("TerminationSemantics violated: returned false (advisory heuristic must always return true)")
		}
	})
}

// TestTimestampDetector_KnownFormatsClaim_Property anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guidance — built from a static corpus of timestamp
//	                formats… The detection condition: probability
//	                strictly greater than match_threshold.
//	    @guarantee LabelDomain — claim emits start_group
//
// Directed property test: for every format in the static
// knownTimestampFormats corpus that tokenizes to at least
// minimumTokenLength tokens, the detector — with the default
// match_threshold — claims and emits start_group. This is the
// inverse of LabelDomain_Property: instead of "if claimed, label
// is start_group", we verify "for known-good inputs, the detector
// DOES claim and the claim is start_group." Catches detector
// misconfiguration (threshold raised too high, corpus drift,
// tokenizer mismatch) that would silently degrade accuracy.
//
// The minimumTokenLength filter is load-bearing: the corpus is
// used to *build* the TokenGraph, not as a self-match dataset.
// Some short formats (e.g. "11:42:35.173") deliberately
// contribute tokens to the graph without themselves clearing the
// minimum-length gate. Excluding those from the directed test
// reflects the design: the property is "long enough formats
// match", not "every entry in the corpus matches."
//
// The filtered corpus is rapid-sampled, which prints the failing
// input on shrink — useful when adding new formats to identify
// which addition breaks the calibration.
func TestTimestampDetector_KnownFormatsClaim_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	tok := NewTokenizer(mockConfig.GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes"))
	detector := NewTimestampDetector(mockConfig.GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold"))

	// Pre-filter to formats whose tokenization clears the
	// minimum-length gate. Formats below the gate produce
	// probability 0 from MatchProbability by construction —
	// excluding them isolates the actual matching-accuracy
	// property from the engine's well-defined "too short to
	// evaluate" lower bound.
	var matchableFormats []string
	for _, format := range knownTimestampFormats {
		tokens, _ := tok.Tokenize([]byte(format))
		if len(tokens) >= minimumTokenLength {
			matchableFormats = append(matchableFormats, format)
		}
	}
	if len(matchableFormats) == 0 {
		t.Fatal("no formats in knownTimestampFormats tokenize to at least minimumTokenLength — calibration is wholly broken")
	}

	rapid.Check(t, func(t *rapid.T) {
		format := rapid.SampledFrom(matchableFormats).Draw(t, "format")
		content := []byte(format)
		tokens, indices := tok.Tokenize(content)
		ctx := &messageContext{
			rawMessage:      content,
			tokens:          tokens,
			tokenIndicies:   indices,
			label:           aggregate,
			labelAssignedBy: defaultLabelSource,
		}
		detector.ProcessAndContinue(ctx)
		if ctx.label != startGroup {
			t.Fatalf("known timestamp format %q (tokens=%d, >= minimumTokenLength=%d) failed to claim: label is %v (expected start_group)",
				format, len(tokens), minimumTokenLength, ctx.label)
		}
	})
}
