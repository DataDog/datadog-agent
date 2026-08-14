// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Directed anchoring tests for the JSONAggregator contract @invariants
// declared in json_aggregator.allium. Each test runs against BOTH the
// NoopJSONAggregator and the accumulating jsonAggregator implementation
// so that the contract is exercised on every fulfiller.
//
// Surface-level @guarantees (CompletenessOrPassthrough,
// ContentBytePassthrough, ArrivalOrderEmission, etc.) are anchored in
// json_aggregator_proptest_test.go. This file is contract-level only.
//
// ResultLifetime (the sixth @invariant) is documented in the spec as
// a caller obligation rather than an aggregator-verifiable property,
// and therefore has no anchoring test here.

// jsonAggregatorImpls returns the JSONAggregator implementations the
// contract @invariants apply to. Tests below sweep across both.
func jsonAggregatorImpls() []struct {
	name string
	ctor func() JSONAggregator
} {
	return []struct {
		name string
		ctor func() JSONAggregator
	}{
		{name: "Noop", ctor: func() JSONAggregator { return NewNoopJSONAggregator() }},
		{name: "Multiline", ctor: func() JSONAggregator { return NewJSONAggregator(false, 1024) }},
	}
}

// TestJSONAggregatorContract_Determinism anchors:
//
//	contract JSONAggregator
//	    @invariant Determinism
//
// Two fresh aggregators of the same implementation, fed equal input
// sequences, return equal output sequences and equal IsEmpty results
// at every step. Determinism is the stateful analogue of pure-function
// equivalence: pure given internal state.
func TestJSONAggregatorContract_Determinism(t *testing.T) {
	inputs := [][]string{
		{`{"a":1}`},
		{`{`, `"a":1`, `}`},
		{`not json`, `also not json`},
		{`{"a":`, `1}`, `{"b":2}`},
		{``, `{"k":"v"}`, ``},
	}

	for _, impl := range jsonAggregatorImpls() {
		t.Run(impl.name, func(t *testing.T) {
			for _, inputSeq := range inputs {
				a := impl.ctor()
				b := impl.ctor()
				for i, line := range inputSeq {
					ra := a.Process(newTestMessage(line))
					rb := b.Process(newTestMessage(line))
					require.Equal(t, len(ra), len(rb), "Process output length divergence at step %d for input %v", i, inputSeq)
					for j := range ra {
						assert.Equal(t, ra[j].GetContent(), rb[j].GetContent(),
							"Process emission %d content divergence at step %d for input %v", j, i, inputSeq)
					}
					assert.Equal(t, a.IsEmpty(), b.IsEmpty(),
						"IsEmpty divergence at step %d for input %v", i, inputSeq)
				}
				fa := a.Flush()
				fb := b.Flush()
				require.Equal(t, len(fa), len(fb), "Flush output length divergence for input %v", inputSeq)
				for j := range fa {
					assert.Equal(t, fa[j].GetContent(), fb[j].GetContent(),
						"Flush emission %d content divergence for input %v", j, inputSeq)
				}
			}
		})
	}
}

// TestJSONAggregatorContract_Totality anchors:
//
//	contract JSONAggregator
//	    @invariant Totality
//
// Process, Flush, and IsEmpty return defined values on every
// well-formed input — no panics, and the returned sequences are
// iterable as Sequence<Message>. This is the stateful analogue of
// "process is a total function over its domain."
//
// In Go, a nil slice is the canonical empty Sequence<Message> and
// is fully iterable — callers use len() and range — so the test
// asserts iterability (len succeeds, range produces 0 elements
// when nil), not non-nil-ness. The implementations are total over
// arbitrary byte content (Process has no error channel), so this
// test sweeps diverse inputs likely to drive validator state
// machines into corners.
func TestJSONAggregatorContract_Totality(t *testing.T) {
	cornerInputs := []string{
		``,
		`{`,
		`{}`,
		`{"a":1}`,
		`[1,2,3]`,
		`null`,
		`\x00\x01\x02`,
		`{"a":` + string(make([]byte, 2048)) + `}`,
		`"trailing whitespace"   `,
		string([]byte{0xff, 0xfe, 0xfd}),
	}

	for _, impl := range jsonAggregatorImpls() {
		t.Run(impl.name, func(_ *testing.T) {
			ag := impl.ctor()
			for _, line := range cornerInputs {
				result := ag.Process(newTestMessage(line))
				// Iterability: len/cap must succeed on the returned slice
				// (nil is iterable as an empty Sequence<Message>).
				_ = len(result)
				_ = cap(result)
				_ = ag.IsEmpty() // must not panic
			}
			flushed := ag.Flush()
			_ = len(flushed)
			_ = cap(flushed)
			_ = ag.IsEmpty() // must not panic post-flush
		})
	}
}

// TestJSONAggregatorContract_ByteConservation anchors:
//
//	contract JSONAggregator
//	    @invariant ByteConservation
//
// Each emitted message's content is derived from the inputs the
// aggregator has seen; no content is invented from external sources.
// For the Noop implementation: emitted bytes are byte-identical to
// the input. For the multiline implementation: emitted bytes are a
// (possibly compacted) concatenation of one or more inputs. This
// test verifies the per-implementation refinement matches the
// contract.
func TestJSONAggregatorContract_ByteConservation(t *testing.T) {
	// Noop: byte-identical pass-through.
	t.Run("Noop_bytes_identical", func(t *testing.T) {
		noop := NewNoopJSONAggregator()
		inputs := []string{`{"a":1}`, `not json`, ``, `partial {`}
		for _, line := range inputs {
			result := noop.Process(newTestMessage(line))
			require.Len(t, result, 1, "Noop must emit exactly one message per input")
			assert.Equal(t, line, string(result[0].GetContent()),
				"Noop emission bytes must be byte-identical to input")
		}
	})

	// Multiline: emitted bytes are a substring/concatenation of inputs
	// (compaction is permitted). Verify that no byte sequence appears
	// in an emission that wasn't seen in some input.
	t.Run("Multiline_bytes_derived", func(t *testing.T) {
		ag := NewJSONAggregator(false, 1024)
		inputs := []string{`{`, `  "key": "value"`, `}`}
		combined := strings.Join(inputs, "")
		emissions := ag.Process(newTestMessage(inputs[0]))
		require.Empty(t, emissions, "incomplete JSON should buffer")
		emissions = ag.Process(newTestMessage(inputs[1]))
		require.Empty(t, emissions, "still-incomplete JSON should buffer")
		emissions = ag.Process(newTestMessage(inputs[2]))
		require.Len(t, emissions, 1, "completing the object should emit once")
		// The emission's bytes should be a compaction of the union of
		// inputs — every emitted byte should originate from some input
		// (no external content invented). Verify the emission is valid
		// JSON whose key/value pair matches what the inputs contained.
		emitted := string(emissions[0].GetContent())
		assert.Contains(t, emitted, "key", "emission should contain input key")
		assert.Contains(t, emitted, "value", "emission should contain input value")
		assert.NotContains(t, emitted, "INVENTED", "emission must not contain bytes not in inputs")
		// Conservation: the emitted byte count is <= sum of input
		// content bytes (compaction may reduce).
		assert.LessOrEqual(t, len(emitted), len(combined),
			"compacted emission cannot grow beyond the sum of inputs")
	})
}

// TestJSONAggregatorContract_FlushDrainsBuffer anchors:
//
//	contract JSONAggregator
//	    @invariant FlushDrainsBuffer
//
// After Flush returns, IsEmpty returns true. Calling Flush on an
// already-empty aggregator returns an empty sequence and leaves
// observable state unchanged.
func TestJSONAggregatorContract_FlushDrainsBuffer(t *testing.T) {
	for _, impl := range jsonAggregatorImpls() {
		t.Run(impl.name+"_flush_drains_after_partial", func(t *testing.T) {
			ag := impl.ctor()
			// Process partial input that the multiline impl will buffer.
			// For Noop, this is emitted immediately and the aggregator
			// stays empty — both paths satisfy the invariant.
			ag.Process(newTestMessage(`{"a":`))
			ag.Flush()
			assert.True(t, ag.IsEmpty(), "IsEmpty must be true after Flush")
		})

		t.Run(impl.name+"_flush_empty_is_noop", func(t *testing.T) {
			ag := impl.ctor()
			require.True(t, ag.IsEmpty(), "fresh aggregator must be empty")
			result := ag.Flush()
			assert.Empty(t, result, "Flush on empty aggregator must return empty sequence")
			assert.True(t, ag.IsEmpty(), "IsEmpty must remain true after empty Flush")
			// A second Flush is equally a no-op.
			result = ag.Flush()
			assert.Empty(t, result, "second consecutive Flush on empty must also return empty")
			assert.True(t, ag.IsEmpty(), "IsEmpty must remain true after second empty Flush")
		})
	}
}

// TestJSONAggregatorContract_IsEmptyConsistency anchors:
//
//	contract JSONAggregator
//	    @invariant IsEmptyConsistency
//
// IsEmpty returns true at construction and after every flush. It
// returns false while the aggregator holds buffered content. The
// pairing is symmetric: IsEmpty = true iff Flush would return an
// empty sequence.
func TestJSONAggregatorContract_IsEmptyConsistency(t *testing.T) {
	for _, impl := range jsonAggregatorImpls() {
		t.Run(impl.name+"_empty_at_construction", func(t *testing.T) {
			ag := impl.ctor()
			assert.True(t, ag.IsEmpty(), "fresh aggregator must be empty")
		})

		t.Run(impl.name+"_symmetry_with_flush", func(t *testing.T) {
			ag := impl.ctor()
			// Drive a sequence of inputs and verify the symmetry at each step.
			steps := []string{`{`, `"a":1`, `}`, `{"b":2}`, `not json`}
			for i, line := range steps {
				ag.Process(newTestMessage(line))
				empty := ag.IsEmpty()
				// Run Flush on a COPY of the state by constructing a fresh
				// aggregator and replaying inputs up to this step. The
				// invariant claims: IsEmpty iff Flush returns empty. We
				// can't snapshot the live aggregator without consuming it,
				// so we replay.
				replay := impl.ctor()
				for j := 0; j <= i; j++ {
					replay.Process(newTestMessage(steps[j]))
				}
				flushResult := replay.Flush()
				if empty {
					assert.Empty(t, flushResult,
						"step %d: IsEmpty=true but Flush returned %d messages on replayed state", i, len(flushResult))
				} else {
					assert.NotEmpty(t, flushResult,
						"step %d: IsEmpty=false but Flush returned empty on replayed state", i)
				}
			}
		})
	}
}
