// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package json

import (
	"bytes"
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
		err := PrintJSON(&buf, testData, false, true)
		require.NoError(t, err)

		output := buf.String()
		// Should be single line (plus newline from Fprintln)
		assert.Equal(t, 1, strings.Count(output, "\n"))
		assert.Contains(t, output, `"name":"test"`)
	})

	t.Run("pretty output", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintJSON(&buf, testData, true, true)
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
		err := PrintJSON(&buf, invalidData, false, true)
		assert.Error(t, err)
		// Error comes directly from json.Marshal
		assert.Contains(t, err.Error(), "json")
	})
}
