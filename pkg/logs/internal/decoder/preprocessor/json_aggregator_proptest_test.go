// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// The properties in this file are derived from named constructs in
// pkg/logs/internal/decoder/preprocessor/json_aggregator.allium. Each
// test names the spec construct it anchors so that drift in either
// direction is easy to spot during review.

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

// TestValidator_Determinism anchors:
//
//	contract IncrementalJSONValidator
//	    @invariant Determinism
//
// Two fresh validators driven with the same byte sequence must return
// identical states at every step. This is the property that lets
// downstream rules treat the validator as a pure function of its
// accumulated input.
func TestValidator_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		chunks := rapid.SliceOfN(
			rapid.SliceOfN(rapid.Byte(), 0, 32),
			1, 8,
		).Draw(t, "chunks")

		v1 := NewIncrementalJSONValidator()
		v2 := NewIncrementalJSONValidator()
		for i, c := range chunks {
			s1 := v1.Write(c)
			s2 := v2.Write(c)
			if s1 != s2 {
				t.Fatalf("non-deterministic at chunk %d: v1=%v v2=%v on %q",
					i, s1, s2, c)
			}
		}
	})
}

// TestValidator_InvalidStable anchors:
//
//	contract IncrementalJSONValidator
//	    @invariant InvalidStable
//
// Once the validator returns Invalid for some accumulated input, any
// extension also returns Invalid. The aggregator relies on this to
// treat an invalid prefix as terminal until reset.
//
// Random byte input is overwhelmingly invalid JSON, so this property
// exercises the dominant input distribution. Skipping when the head
// is not Invalid is intentional: the invariant only constrains
// extensions of already-invalid inputs.
func TestValidator_InvalidStable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		head := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "head")
		tail := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "tail")

		v := NewIncrementalJSONValidator()
		if v.Write(head) != Invalid {
			t.Skip("head did not produce Invalid; invariant only constrains extensions of invalid prefixes")
		}

		if got := v.Write(tail); got != Invalid {
			t.Fatalf("InvalidStable violated: head was Invalid but extension produced %v (head=%q tail=%q)",
				got, head, tail)
		}
	})
}

// TestValidator_TopLevelArrayInvalid anchors:
//
//	contract IncrementalJSONValidator
//	    @invariant TopLevelArrayInvalid
//
// A top-level JSON array — empty, scalar-element, object-element
// or string-element — must be reported as Invalid by the
// IncrementalJSONValidator, even though arrays are syntactically
// valid JSON under a general-purpose RFC 8259 validator. This
// invariant is what causes the TopLevelArrayNotAggregated
// limitation captured at the bottom of the spec: the moment the
// validator sees an opening "[" at the top level it commits to
// Invalid and the aggregator flushes.
//
// Top-level scalars (strings, numbers, booleans, null) are NOT
// asserted here. The spec's contract @guidance documents that the
// validator does not reject those — they fall through as Complete
// due to the tokenizer-based implementation, and the aggregator's
// fast path catches them before the validator sees them in normal
// operation. Spec consumers must not depend on validator-level
// rejection of scalars.
//
// Each row below is a representative of one array shape. The table
// is the property generator: every row must satisfy the same
// post-condition, regardless of element content.
func TestValidator_TopLevelArrayInvalid(t *testing.T) {
	arrays := []string{
		`[]`,
		`[1,2,3]`,
		`[{"a":1}]`,
		`["a","b"]`,
		`[null]`,
		`[true,false]`,
	}

	for _, in := range arrays {
		t.Run(in, func(t *testing.T) {
			v := NewIncrementalJSONValidator()
			got := v.Write([]byte(in))
			assert.Equal(t, Invalid, got,
				"top-level JSON array %q must be Invalid (TopLevelArrayInvalid invariant in json_aggregator.allium)",
				in)
		})
	}
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
