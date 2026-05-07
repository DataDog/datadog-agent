// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// jsonKey generates a non-empty lowercase ASCII string suitable as a
// JSON object key. Kept separate from safeString so the alphabet
// excludes spaces and punctuation that would survive json.Marshal but
// add irrelevant variation to the test space.
func jsonKey() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		bs := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdefghijklmnopqrstuvwxyz")),
			1, 6,
		).Draw(t, "keyBytes")
		return string(bs)
	})
}

// jsonStrValue generates a JSON-safe string value (lowercase ASCII +
// digits + space). Avoids quotes, backslashes and control characters
// so json.Marshal's escaping does not produce surprising bytes that
// trip up the byte-equality assertion.
func jsonStrValue() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		bs := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdefghijklmnopqrstuvwxyz0123456789 ")),
			0, 12,
		).Draw(t, "valBytes")
		return string(bs)
	})
}

// jsonObject generates a flat top-level JSON object with up to 5 keys
// and scalar values (string, integer, boolean, null). Flat structure
// is sufficient for exercising EmitAggregated; nested objects are a
// stronger generator that can be added later if a regression motivates
// it.
func jsonObject() *rapid.Generator[map[string]interface{}] {
	return rapid.Custom(func(t *rapid.T) map[string]interface{} {
		n := rapid.IntRange(0, 5).Draw(t, "numKeys")
		m := make(map[string]interface{}, n)
		for i := 0; i < n; i++ {
			key := jsonKey().Draw(t, fmt.Sprintf("k%d", i))
			switch rapid.IntRange(0, 3).Draw(t, fmt.Sprintf("kind%d", i)) {
			case 0:
				m[key] = nil
			case 1:
				m[key] = rapid.Bool().Draw(t, fmt.Sprintf("b%d", i))
			case 2:
				m[key] = rapid.IntRange(-1000, 1000).Draw(t, fmt.Sprintf("n%d", i))
			default:
				m[key] = jsonStrValue().Draw(t, fmt.Sprintf("s%d", i))
			}
		}
		return m
	})
}

// TestAggregator_EmitAggregatedPreservesContent anchors:
//
//	surface JSONAggregation
//	    @guarantee ContentBytePassthrough
//
// on the EmitAggregated rule path specifically. The existing
// TestAggregator_FlushPathsPreserveBytes covers the flush paths
// (FlushOnInvalid, FlushOnSizeLimit, DrainOnFlush) and the
// FastPathEmit fall-through, but explicitly excludes EmitAggregated by
// generator construction. This test closes that gap.
//
// The property: when a valid top-level JSON object is split across two
// or more Process calls and the aggregator runs the EmitAggregated
// rule, the emitted bytes equal json.Compact applied to the
// concatenation of the input chunks. Spec wording: "the only
// modification is deterministic JSON compaction (whitespace elision)
// of the concatenated buffered content."
//
// Generator strategy:
//
//   - Generate a flat top-level JSON object with scalar values.
//   - Marshal it with indentation so the input has insignificant
//     whitespace for compaction to strip; otherwise the assertion
//     would only catch concatenation bugs, not whitespace handling.
//   - Split into exactly 2 chunks at the byte immediately after the
//     opening "{". This is a safe split position: the validator
//     handles the boundary between an opening object delimiter and
//     the rest of the body cleanly. Splitting mid-token (e.g. inside
//     a string literal) is not safe — Go's json.Decoder does not
//     reliably resume Token() reads after ErrUnexpectedEOF in the
//     middle of a token. That limitation is a separate behavioural
//     question and deserves its own test if/when the spec promises
//     anything about it.
//
// FragmentBackrefConsistent and FlushOnSizeLimit are intentionally not
// tested here; max_content_size is generous enough that the size-limit
// transition does not fire.
func TestAggregator_EmitAggregatedPreservesContent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		obj := jsonObject().Draw(t, "obj")

		pretty, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			t.Fatalf("json.MarshalIndent failed on generated object: %v", err)
		}
		if len(pretty) < 2 || pretty[0] != '{' {
			t.Fatalf("generator produced unexpected JSON shape: %q", pretty)
		}

		// Always split right after the opening "{" — a safe boundary
		// that exercises the multi-message EmitAggregated path without
		// tripping mid-token resumption issues in json.Decoder.
		chunk1 := pretty[:1]
		chunk2 := pretty[1:]

		var compacted bytes.Buffer
		if err := json.Compact(&compacted, pretty); err != nil {
			t.Fatalf("json.Compact failed on generated object: %v", err)
		}
		expected := compacted.String()

		// tag_complete_json=false to keep the byte-equality assertion
		// focused on content; tag presence is a separate property.
		agg := NewJSONAggregator(false, 1_000_000)

		emitted1 := agg.Process(newTestMessage(string(chunk1)))
		if len(emitted1) != 0 {
			t.Fatalf("expected no emission after first chunk (incomplete prefix), got %d emissions; chunk1=%q",
				len(emitted1), chunk1)
		}

		emitted2 := agg.Process(newTestMessage(string(chunk2)))
		if len(emitted2) != 1 {
			t.Fatalf("expected exactly one emission after second chunk completes the object, got %d emissions; chunks=%q,%q",
				len(emitted2), chunk1, chunk2)
		}

		actual := string(emitted2[0].GetContent())
		if actual != expected {
			t.Fatalf("EmitAggregated byte preservation violated:\n  input    = %q\n  expected = %q\n  actual   = %q",
				pretty, expected, actual)
		}
	})
}
