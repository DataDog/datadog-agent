// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cells follow intake INPUT-01 and INPUT-07.
func TestCellsHoldOneJSONValue(t *testing.T) {
	for _, ok := range []string{"0", "-0", "7", "-12", "0.5", "12345678901234567890.123456789012345678", "1E+25", "1e-7", strings.Repeat("9", 126)} {
		assert.True(t, ValidJSONNumber(ok), ok)
		c, valid := DecimalCell(ok)
		require.True(t, valid)
		assert.Equal(t, ok, c.Token, "a decimal is its exact text")
		assert.GreaterOrEqual(t, c.Bound, len(ok)+2, "intake stores it as a string")
	}
	for _, bad := range []string{"", "-", "+1", ".5", "5.", "01", "1e", "NaN", "Infinity", "0x10", "1,5", "1 ", strings.Repeat("9", 201)} {
		assert.False(t, ValidJSONNumber(bad), bad)
		_, valid := DecimalCell(bad)
		assert.False(t, valid, bad)
	}
	for _, special := range []string{"NaN", "Infinity", "-Infinity"} {
		c, valid := FloatCell(special)
		require.True(t, valid)
		assert.Equal(t, `"`+special+`"`, c.Token)
	}
	c, valid := FloatCell("1.25")
	require.True(t, valid)
	assert.Equal(t, "1.25", c.Token)
	assert.GreaterOrEqual(t, c.Bound, 24, "intake may write a float back longer")

	c, valid = StringCell("<&>\u2028é\x00")
	require.True(t, valid)
	var back string
	require.NoError(t, json.Unmarshal([]byte(c.Token), &back))
	assert.Equal(t, "<&>\u2028é\x00", back)
	assert.NotContains(t, c.Token, `\u003c`)
	c, _ = StringCell("ab")
	assert.Equal(t, redactedTokenBytes, c.Bound, "a short value can be replaced by the longer redaction marker")
	_, valid = StringCell("bad\xff")
	assert.False(t, valid)
	assert.Equal(t, Cell{Token: "null", Bound: 4}, Null)
}

// DESC-03
func TestDESC03_RejectResultsADescriptorCannotCarry(t *testing.T) {
	lim := Limits{MaxColumns: 2, MaxSchemaBytes: 1 << 20}
	col := func(name string) Column {
		return Column{Name: name, DatabaseType: "NUMBER", LogicalType: LogicalDecimal}
	}
	assert.Nil(t, ValidateColumns([]Column{col("A"), col("B")}, lim, true))
	for name, tc := range map[string]struct {
		cols   []Column
		limits Limits
		schema bool
		code   string
	}{
		"no columns":       {nil, lim, false, CodeInvalidColumns},
		"too many":         {[]Column{col("A"), col("B"), col("C")}, lim, false, CodeInvalidColumns},
		"duplicate":        {[]Column{col("A"), col("A")}, lim, false, CodeInvalidColumns},
		"empty name":       {[]Column{col("")}, lim, false, CodeInvalidColumns},
		"long name":        {[]Column{col(strings.Repeat("c", 256))}, lim, false, CodeInvalidColumns},
		"invalid UTF-8":    {[]Column{col("bad\xff")}, lim, false, CodeInvalidColumns},
		"schema too large": {[]Column{col("A")}, Limits{MaxColumns: 2, MaxSchemaBytes: 10}, true, CodeSchemaTooLarge},
	} {
		f := ValidateColumns(tc.cols, tc.limits, tc.schema)
		require.NotNil(t, f, name)
		assert.Equal(t, tc.code, f.Code, name)
	}
	f := ValidateColumns([]Column{col("TOTAL"), col("TOTAL")}, lim, false)
	assert.Contains(t, f.Message, `"TOTAL"`, "the error names the duplicate column")
	assert.Nil(t, ValidateColumns([]Column{col("A")}, Limits{MaxColumns: 2, MaxSchemaBytes: 10}, false), "the schema size only matters when it is included")
}
