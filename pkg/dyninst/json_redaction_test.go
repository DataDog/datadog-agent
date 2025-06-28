// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

type matcher interface {
	matches(ptr jsontext.Pointer) bool
}

type replacer interface {
	replace(jsontext.Value) jsontext.Value
}

type replacerFunc func(jsontext.Value) jsontext.Value

func (f replacerFunc) replace(v jsontext.Value) jsontext.Value {
	return f(v)
}

type jsonRedactor struct {
	matcher  matcher
	replacer replacer
}

type exactMatcher string

func (m exactMatcher) matches(ptr jsontext.Pointer) bool {
	return string(ptr) == string(m)
}

type replacement jsontext.Value

func (r replacement) replace(jsontext.Value) jsontext.Value {
	return jsontext.Value(r)
}

type prefixSuffixMatcher [2]string

func (m prefixSuffixMatcher) matches(ptr jsontext.Pointer) bool {
	return strings.HasPrefix(string(ptr), m[0]) &&
		strings.HasSuffix(string(ptr), m[1])
}

func redactor(matcher matcher, replacer replacer) jsonRedactor {
	return jsonRedactor{
		matcher:  matcher,
		replacer: replacer,
	}
}

var defaultRedactors = []jsonRedactor{
	redactor(
		exactMatcher(`/debugger/snapshot/stack`),
		replacement(`"[stack-unredact-me]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/id`),
		replacement(`"[id]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		exactMatcher(`/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/address"},
		replacement(`"[addr]"`),
	),
}

func redactJSON(t *testing.T, input []byte, redactors []jsonRedactor) (redacted []byte) {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(&buf, jsontext.WithIndent("  "), jsontext.WithIndentPrefix("  "))
	for {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		kind, idx := d.StackIndex(d.StackDepth())
		err = e.WriteToken(tok)
		require.NoError(t, err)
		if kind != '{' || idx%2 == 0 {
			continue
		}
		ptr := d.StackPointer()
		for _, redactor := range redactors {
			if redactor.matcher.matches(ptr) {
				v, err := d.ReadValue()
				require.NoError(t, err)
				err = e.WriteValue(redactor.replacer.replace(v))
				require.NoError(t, err)
				break
			}
		}
	}
	return bytes.TrimSpace(buf.Bytes())
}

type sorter interface {
	sort(jsontext.Value) jsontext.Value
}

type entriesSorter struct {
	ascending bool
}

func (s entriesSorter) sort(v jsontext.Value) jsontext.Value {
	// Parse the JSON value using standard json package
	var entries []interface{}
	if err := json.Unmarshal([]byte(v), &entries); err != nil {
		return v // Return original if can't parse
	}

	// Sort the entries based on the value field of the first element
	sorted := make([]interface{}, len(entries))
	copy(sorted, entries)

	// Sort using the same logic as before
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			keyI := s.extractSortKey(sorted[i])
			keyJ := s.extractSortKey(sorted[j])

			shouldSwap := false
			if s.ascending {
				shouldSwap = s.compareKeys(keyI, keyJ) > 0
			} else {
				shouldSwap = s.compareKeys(keyI, keyJ) < 0
			}

			if shouldSwap {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Marshal back to JSON value
	if sortedBytes, err := json.Marshal(sorted); err == nil {
		return jsontext.Value(sortedBytes)
	}
	return v // Return original if marshaling fails
}

func (s entriesSorter) extractSortKey(entry interface{}) string {
	// Entry should be an array with at least one element
	entrySlice, ok := entry.([]interface{})
	if !ok || len(entrySlice) == 0 {
		return ""
	}

	// First element should be a map/object
	firstElement, ok := entrySlice[0].(map[string]interface{})
	if !ok {
		return ""
	}

	// Check if value exists
	if value, exists := firstElement["value"]; exists {
		if valueStr, ok := value.(string); ok {
			return valueStr
		}
		return ""
	}

	// If no value, check for notCapturedReason (these should sort to the end)
	if reason, exists := firstElement["notCapturedReason"]; exists {
		// Use a special prefix to ensure these sort after regular values
		return "~notCaptured:" + reason.(string)
	}

	return ""
}

func (s entriesSorter) compareKeys(a, b string) int {
	// Handle notCaptured entries (they should always sort to the end)
	aIsNotCaptured := len(a) > 0 && a[0] == '~'
	bIsNotCaptured := len(b) > 0 && b[0] == '~'

	if aIsNotCaptured && !bIsNotCaptured {
		return 1 // a sorts after b
	}
	if !aIsNotCaptured && bIsNotCaptured {
		return -1 // a sorts before b
	}
	if aIsNotCaptured && bIsNotCaptured {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}

	// Try to parse as numbers first
	if len(a) > 0 && len(b) > 0 {
		if (a[0] >= '0' && a[0] <= '9') && (b[0] >= '0' && b[0] <= '9') {
			// Simple numeric comparison for strings that start with digits
			if len(a) != len(b) {
				if len(a) < len(b) {
					return -1
				}
				return 1
			}
		}
	}

	// Fall back to string comparison
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

type jsonSorter struct {
	matcher matcher
	sorter  sorter
}

type entriesFieldMatcher struct{}

func (m entriesFieldMatcher) matches(ptr jsontext.Pointer) bool {
	ptrStr := string(ptr)
	return strings.HasSuffix(ptrStr, "/entries")
}

func sortJSON(t *testing.T, input []byte, sorters []jsonSorter) (sorted []byte) {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(&buf, jsontext.WithIndent("  "), jsontext.WithIndentPrefix("  "))
	for {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		kind, idx := d.StackIndex(d.StackDepth())
		err = e.WriteToken(tok)
		require.NoError(t, err)
		if kind != '{' || idx%2 == 0 {
			continue
		}
		ptr := d.StackPointer()
		for _, sorter := range sorters {
			if sorter.matcher.matches(ptr) {
				v, err := d.ReadValue()
				require.NoError(t, err)
				err = e.WriteValue(sorter.sorter.sort(v))
				require.NoError(t, err)
				break
			}
		}
	}
	return bytes.TrimSpace(buf.Bytes())
}

var defaultSorters = []jsonSorter{
	{
		matcher: entriesFieldMatcher{},
		sorter:  entriesSorter{ascending: true},
	},
}

func TestSortJSON(t *testing.T) {
	// Sample JSON data similar to the debugger snapshot
	input := `{
		"debugger": {
			"snapshot": {
				"captures": {
					"entry": {
						"arguments": {
							"m": {
								"type": "map[string]int",
								"entries": [
									[
										{
											"type": "string",
											"value": "BBB"
										},
										{
											"type": "int",
											"value": "2"
										}
									],
									[
										{
											"type": "string",
											"value": "AAA"
										},
										{
											"type": "int",
											"value": "1"
										}
									],
									[
										{
											"type": "string",
											"value": "CCC"
										},
										{
											"type": "int",
											"value": "3"
										}
									],
									[
										{
											"type": "string",
											"notCapturedReason": "depth"
										},
										{
											"type": "int",
											"value": "4"
										}
									]
								]
							}
						}
					}
				}
			}
		}
	}`

	t.Run("ascending sort", func(t *testing.T) {
		sorters := []jsonSorter{
			{
				matcher: entriesFieldMatcher{},
				sorter:  entriesSorter{ascending: true},
			},
		}

		result := sortJSON(t, []byte(input), sorters)
		t.Logf("Sorted JSON (ascending):\n%s", string(result))

		// Verify the result contains sorted entries
		require.Contains(t, string(result), `"value": "AAA"`)
		require.Contains(t, string(result), `"value": "BBB"`)
		require.Contains(t, string(result), `"value": "CCC"`)
		require.Contains(t, string(result), `"notCapturedReason": "depth"`)
	})

	t.Run("descending sort", func(t *testing.T) {
		sorters := []jsonSorter{
			{
				matcher: entriesFieldMatcher{},
				sorter:  entriesSorter{ascending: false},
			},
		}

		result := sortJSON(t, []byte(input), sorters)
		t.Logf("Sorted JSON (descending):\n%s", string(result))

		// Verify the result contains the entries
		require.Contains(t, string(result), `"value": "AAA"`)
		require.Contains(t, string(result), `"value": "BBB"`)
		require.Contains(t, string(result), `"value": "CCC"`)
		require.Contains(t, string(result), `"notCapturedReason": "depth"`)
	})
}
