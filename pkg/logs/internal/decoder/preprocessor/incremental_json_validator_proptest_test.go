// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// Property tests for the IncrementalJSONValidator contract declared in
// pkg/logs/internal/decoder/preprocessor/json_aggregator.allium. Each test
// names the contract @invariant it anchors so that drift in either
// direction is easy to spot during review.
//
// Anchoring tests for the validator live in
// incremental_json_validator_test.go.

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
// Asserts case-(a) stability: for random Invalid heads whose leading
// non-whitespace byte cannot begin a multi-byte JSON token (i.e. the
// head cannot be a case-(b) prefix per the @invariant), any extension
// also returns Invalid.
//
// The couldBeJSONPrefix filter skips heads that could still be
// completing their leading token (unclosed string, partial keyword,
// number in progress, or leading whitespace). Those states are
// permitted by the weakened @invariant to transition to Complete or
// Incomplete when extended. Filtering them out preserves random
// coverage across the majority of the input space while keeping the
// assertion sound.
//
// The filter over-skips slightly (e.g. "1abc" is case-(a) terminal
// but starts with a digit) — directed unit tests in
// incremental_json_validator_test.go cover those shapes.
func TestValidator_InvalidStable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		head := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "head")
		tail := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "tail")

		v := NewIncrementalJSONValidator()
		if v.Write(head) != Invalid {
			t.Skip("head not Invalid; invariant only constrains extensions of Invalid heads")
		}

		if couldBeJSONPrefix(head) {
			t.Skip("head could be a case-(b) prefix (leading token not yet complete); weakened invariant does not bind")
		}

		if got := v.Write(tail); got != Invalid {
			t.Fatalf("InvalidStable violated (case a): head=%q tail=%q → %v", head, tail, got)
		}
	})
}

// couldBeJSONPrefix returns true if b could be a strict prefix of a
// valid JSON value whose leading token has not yet been completed
// (case (b) of the InvalidStable @invariant). The predicate is
// deliberately conservative — it may return true for inputs that are
// actually case-(a) terminal (e.g. "1abc"). That over-skip is a sound
// direction: it never asserts stability on a genuine case-(b) input.
//
// A byte can begin a multi-byte JSON token if it opens a string ("),
// starts a keyword literal (t, f, n), or begins a number (digit or -).
// Leading whitespace permits any of the above later per RFC 8259 §2.
func couldBeJSONPrefix(b []byte) bool {
	// Advance past leading whitespace.
	i := 0
	for i < len(b) && isJSONWhitespaceByte(b[i]) {
		i++
	}
	if i == len(b) {
		return true // all whitespace — could precede any valid value
	}
	c := b[i]
	return c == '"' || c == 't' || c == 'f' || c == 'n' ||
		(c >= '0' && c <= '9') || c == '-'
}

// isJSONWhitespaceByte returns true if c is JSON whitespace per
// RFC 8259 §2: space (0x20), horizontal tab (0x09), LF (0x0A), CR (0x0D).
func isJSONWhitespaceByte(c byte) bool {
	return c == 0x20 || c == 0x09 || c == 0x0A || c == 0x0D
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

// TestValidator_TristateExclusive anchors:
//
//	contract IncrementalJSONValidator
//	    @invariant TristateExclusive
//
// The spec models the validator with two boolean predicates,
// is_complete_object and is_invalid_object, and asserts they are never
// simultaneously true. When both are false the input is incomplete: a
// strict prefix of a complete top-level JSON object.
//
// The Go implementation encodes this disjoint result space as a single
// tristate JSONState enum {Incomplete, Complete, Invalid}. Returning
// any other value would violate the invariant by failing to express
// the trichotomy. This test asserts every Write call across an
// arbitrary sequence of byte chunks returns exactly one of the three
// known states — defensive against enum-arithmetic bugs and
// unintended state values.
func TestValidator_TristateExclusive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		chunks := rapid.SliceOfN(
			rapid.SliceOfN(rapid.Byte(), 0, 32),
			1, 8,
		).Draw(t, "chunks")

		v := NewIncrementalJSONValidator()
		for i, c := range chunks {
			got := v.Write(c)
			switch got {
			case Incomplete, Complete, Invalid:
				// expected: exactly one of the three tristate values
			default:
				t.Fatalf("TristateExclusive violated at chunk %d: state=%v is not in {Incomplete, Complete, Invalid} (input=%q)",
					i, got, c)
			}
		}
	})
}
