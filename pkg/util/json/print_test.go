// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintJSON(t *testing.T) {
	testData := map[string]interface{}{
		"name":  "test",
		"value": 123,
		"nested": map[string]interface{}{
			"key": "val",
		},
	}

	t.Run("compact output", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintJSON(&buf, testData, false, true, "")
		require.NoError(t, err)

		output := buf.String()
		// Should be single line (plus newline from Fprintln)
		assert.Equal(t, 1, strings.Count(output, "\n"))
		assert.Contains(t, output, `"name":"test"`)
	})

	t.Run("pretty output", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintJSON(&buf, testData, true, true, "")
		require.NoError(t, err)

		output := buf.String()
		// Should have multiple lines with indentation
		assert.Greater(t, strings.Count(output, "\n"), 1)
		assert.Contains(t, output, "  ")
		assert.Contains(t, output, `"name": "test"`)
	})

	t.Run("invalid data", func(t *testing.T) {
		var buf bytes.Buffer
		// channels cannot be marshaled to JSON
		invalidData := make(chan int)
		err := PrintJSON(&buf, invalidData, false, true, "")
		assert.Error(t, err)
		// Error comes directly from json.Marshal
		assert.Contains(t, err.Error(), "json")
	})

	t.Run("remove empty fields from RawMessage", func(t *testing.T) {
		// Simulate what workload-list does: backend returns JSON bytes with empty fields
		rawJSON := []byte(`{
			"name": "test",
			"value": 123,
			"empty_string": "",
			"null_field": null,
			"empty_array": [],
			"empty_object": {},
			"nested": {
				"key": "val",
				"empty": "",
				"null": null
			}
		}`)

		var buf bytes.Buffer
		err := PrintJSON(&buf, json.RawMessage(rawJSON), false, true, "")
		require.NoError(t, err)

		output := buf.String()
		// Should keep non-empty fields
		assert.Contains(t, output, `"name":"test"`)
		assert.Contains(t, output, `"value":123`)
		assert.Contains(t, output, `"nested"`)
		assert.Contains(t, output, `"key":"val"`)

		// Should remove null and empty string fields
		assert.NotContains(t, output, `"empty_string"`)
		assert.NotContains(t, output, `"null_field"`)
		assert.NotContains(t, output, `"empty"`)
		assert.NotContains(t, output, `"null"`)

		// Should preserve empty containers (API contract)
		assert.Contains(t, output, `"empty_array"`)
		assert.Contains(t, output, `"empty_object"`)
	})
}
