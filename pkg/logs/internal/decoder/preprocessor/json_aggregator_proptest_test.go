// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Property tests for the JSONAggregation surface and Aggregation entity
// declared in pkg/logs/internal/decoder/preprocessor/json_aggregator.allium.
// Each test names the spec construct it anchors so that drift in either
// direction is easy to spot during review.
//
// Anchoring tests for the aggregator live in json_aggregator_test.go.
// Property tests for the IncrementalJSONValidator contract live in
// incremental_json_validator_proptest_test.go.

// safeString generates a non-empty string that contains no JSON
// structural characters. Callers use this when they need to construct
// inputs that the IncrementalJSONValidator will reject (driving the
// aggregator down a flush path) without accidentally producing valid
// or in-progress JSON.
func safeString() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		bs := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 -_/.")),
			1, 30,
		).Draw(t, "safeBytes")
		return string(bs)
	})
}

// TestAggregator_FlushPathsPreserveBytes anchors:
//
//	surface JSONAggregation
//	    @guarantee ContentBytePassthrough
//
// restricted to the flush paths (FlushOnInvalid, FlushOnSizeLimit,
// DrainOnFlush) and the FastPathEmit fall-through. EmitAggregated is
// excluded by construction: the input generator emits only ASCII
// letters, digits, spaces and a handful of harmless punctuation, so
// no input contains a "{" and the validator's is_complete_object can
// never return true. Every emission therefore comes from a path the
// spec promises preserves bytes exactly.
//
// The property: concatenating the bytes of every emitted message in
// emission order must equal concatenating the bytes of every input
// message in arrival order. This catches both byte mutation and
// silent drops on the same assertion.
func TestAggregator_FlushPathsPreserveBytes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		contents := rapid.SliceOfN(safeString(), 1, 12).Draw(t, "messages")

		// Use a generous max content size: the property is about
		// byte preservation, not the size-limit transition. A small
		// max would keep us honest about the FlushOnSizeLimit path
		// too, but mixing both makes failures harder to attribute;
		// FlushOnSizeLimit deserves its own anchored test.
		agg := NewJSONAggregator(true, 1_000_000)

		var emitted []string
		for _, c := range contents {
			for _, m := range agg.Process(newTestMessage(c)) {
				emitted = append(emitted, string(m.GetContent()))
			}
		}
		for _, m := range agg.Flush() {
			emitted = append(emitted, string(m.GetContent()))
		}

		expected := strings.Join(contents, "")
		actual := strings.Join(emitted, "")

		if expected != actual {
			t.Fatalf("ContentBytePassthrough violated on flush paths:\n  expected = %q\n  actual   = %q",
				expected, actual)
		}
	})
}

// TestAggregator_StateInvariants anchors:
//
//	invariant BufferedSizeNonNegative
//	    aggregation.buffered_size >= 0
//
//	invariant EmptyImpliesZeroSize
//	    aggregation.is_empty implies aggregation.buffered_size = 0
//
// (The third state invariant — FragmentBackrefConsistent — is
// structurally trivial in the Go implementation: buffered messages live
// in the aggregator's own slice, so the backreference is the data
// structure itself. No separate Go assertion is needed.)
//
// The property: after any sequence of Process and Flush calls with
// arbitrary message content, the aggregator's accumulated state
// respects both invariants. Driving arbitrary content (not just valid
// JSON) exercises every rule path — FastPathEmit, BufferIncomplete,
// EmitAggregated, FlushOnInvalid, FlushOnSizeLimit, DrainOnFlush —
// without requiring the test to model which rule fires when. The
// state invariants must hold across the whole rule set, so the test
// only cares that they hold at every step, not which step produced them.
//
// A small max_content_size keeps FlushOnSizeLimit reachable in the
// generated rotation; otherwise long incomplete-fragment runs would
// always hit DrainOnFlush via the explicit Flush op instead.
func TestAggregator_StateInvariants(t *testing.T) {
	type opKind int
	const (
		opProcess opKind = iota
		opFlush
	)
	type op struct {
		kind    opKind
		content string
	}

	// Bias toward Process so sequences accumulate state; sprinkle Flush
	// to exercise the drain transition. Content is fully arbitrary —
	// most random strings will drive FlushOnInvalid; some will be valid
	// JSON; very few will compose into completable objects. The test is
	// agnostic to which path each op takes.
	opGen := rapid.Custom(func(t *rapid.T) op {
		kind := rapid.SampledFrom([]opKind{opProcess, opProcess, opProcess, opFlush}).Draw(t, "kind")
		if kind == opFlush {
			return op{kind: opFlush}
		}
		return op{kind: opProcess, content: rapid.String().Draw(t, "content")}
	})

	rapid.Check(t, func(t *rapid.T) {
		ops := rapid.SliceOfN(opGen, 1, 50).Draw(t, "ops")

		agg := NewJSONAggregator(true, 100)
		internal, ok := agg.(*jsonAggregator)
		if !ok {
			t.Fatalf("NewJSONAggregator returned %T, expected *jsonAggregator", agg)
		}

		for i, o := range ops {
			switch o.kind {
			case opProcess:
				agg.Process(newTestMessage(o.content))
			case opFlush:
				agg.Flush()
			}

			if internal.currentSize < 0 {
				t.Fatalf("BufferedSizeNonNegative violated after op %d: currentSize=%d",
					i, internal.currentSize)
			}
			if agg.IsEmpty() && internal.currentSize != 0 {
				t.Fatalf("EmptyImpliesZeroSize violated after op %d: IsEmpty=true but currentSize=%d",
					i, internal.currentSize)
			}
		}
	})
}
