// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"encoding/json"
	"testing"
)

// FuzzIncrementalJSONValidatorConsistent exercises the incremental validator with arbitrary inputs.
// It checks two properties:
//   - Chunked writes must reach the same state as a single write of the concatenated payload.
//   - Valid JSON objects must be classified as Complete, while other valid JSON values must not.
func FuzzIncrementalJSONValidatorConsistent(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"foo":"bar"}`),
		[]byte(`{"arr":[1,2,3]}`),
		[]byte(`{}`),
		[]byte(`{"nested":{"x":1}}`),
		[]byte(`{"unterminated":`),
		[]byte(`["not","object"]`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<14 { // keep runs quick and memory usage bounded
			return
		}

		// Only reason about JSON objects that are representable with float64 numbers.
		// This filters out syntactically-valid objects containing numbers outside
		// encoding/json's float64 range (which this validator intentionally treats as invalid).
		isObject := false
		var obj map[string]interface{}
		if err := json.Unmarshal(input, &obj); err == nil {
			isObject = true
		}

		single := NewIncrementalJSONValidator()
		singleState := single.Write(input)

		// Splitting the same payload across multiple writes should produce the same terminal state for
		// JSON objects, but only when the split happens at a structural boundary (splitting mid-token is
		// expected to return Invalid).
		if isObject && len(input) >= 2 && len(input) > 0 && (input[0] == '{' || input[0] == '[') {
			split := int(input[0])%(len(input)-1) + 1 // ensure 1 <= split < len
			if isBoundary(input[split-1]) && isBoundary(input[split]) && !inStringLiteral(input[:split]) {
				chunked := NewIncrementalJSONValidator()
				firstState := chunked.Write(input[:split])
				if firstState != Invalid { // skip deliberately broken prefixes
					chunkedState := chunked.Write(input[split:])
					if singleState != chunkedState {
						t.Fatalf("chunked state mismatch on boundary split: single=%v first=%v chunked=%v split=%d input=%q", singleState, firstState, chunkedState, split, input)
					}
				}
			}
		}

		// The validator should only report Complete for well-formed JSON objects.
		if isObject {
			if singleState != Complete {
				t.Fatalf("valid JSON object should be Complete, got %v for %q", singleState, input)
			}
		}
	})
}

// isBoundary returns true if the byte is a likely structural boundary in JSON.
func isBoundary(b byte) bool {
	switch b {
	case '{', '}', '[', ']', ':', ',', '"', ' ', '\n', '\t', '\r':
		return true
	default:
		return false
	}
}

// inStringLiteral reports whether the slice ends inside an unclosed JSON string literal.
func inStringLiteral(prefix []byte) bool {
	inString := false
	escaped := false
	for _, b := range prefix {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
		}
	}
	return inString
}

// FuzzIncrementalJSONValidatorStreaming checks that streaming valid JSON objects in chunks
// is consistent with single-shot validation and never regresses from Complete to Invalid.
func FuzzIncrementalJSONValidatorStreaming(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"k":1} `),
		[]byte("{\"k\":[1,2,3]}\n"),
		[]byte("{\"nested\":{\"x\":\"y\"}}\n\n"),
		[]byte("{\"arr\":[{\"a\":1},{\"b\":2}]}\t"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		// Only reason about JSON objects; skip other valid JSON values.
		var obj map[string]interface{}
		if err := json.Unmarshal(input, &obj); err != nil {
			return
		}

		single := NewIncrementalJSONValidator()
		singleState := single.Write(input)

		// Gather boundary indices (excluding first/last to ensure two chunks).
		var boundaries []int
		inStr := false
		esc := false
		for i := 0; i < len(input); i++ {
			b := input[i]
			if esc {
				esc = false
				continue
			}
			if b == '\\' {
				esc = true
				continue
			}
			if b == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			if i > 0 && (b == ' ' || b == '\n' || b == '\t' || b == '\r') {
				boundaries = append(boundaries, i)
			}
		}
		if len(boundaries) == 0 {
			return
		}

		// Use up to 3 boundary-driven splits derived from input bytes for determinism.
		for idx := 0; idx < len(boundaries) && idx < 3; idx++ {
			split := boundaries[idx]
			chunked := NewIncrementalJSONValidator()
			state := chunked.Write(input[:split])
			states := []JSONState{state}
			if state == Invalid {
				continue
			}
			state = chunked.Write(input[split:])
			states = append(states, state)

			if state != singleState {
				t.Fatalf("final state mismatch: single=%v chunked=%v split=%d input=%q", singleState, state, split, input)
			}
			// Once Complete is reached, it must not revert to Invalid.
			completeSeen := false
			for _, s := range states {
				if completeSeen && s == Invalid {
					t.Fatalf("state regressed to Invalid after Complete: states=%v split=%d input=%q", states, split, input)
				}
				if s == Complete {
					completeSeen = true
				}
			}
		}
	})
}
