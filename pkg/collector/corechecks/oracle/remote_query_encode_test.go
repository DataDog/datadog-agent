// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"strings"
	"testing"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidJSONNumber(t *testing.T) {
	for _, ok := range []string{"0", "-0", "7", "-12", "0.5", "-0.5", "12345678901234567890.123456789012345678", "1E+25", "1e-7", "1.5E3", strings.Repeat("9", 126)} {
		assert.True(t, validJSONNumber(ok), ok)
	}
	for _, bad := range []string{"", "-", "+1", ".5", "5.", "01", "-01", "1e", "1e+", "NaN", "Infinity", "0x10", "1,5", "1 ", " 1", "1_000", strings.Repeat("9", 201)} {
		assert.False(t, validJSONNumber(bad), bad)
	}
}

func TestDriverNumberTextIsLosslessAndJSONValid(t *testing.T) {
	// go-ora decodes a NUMBER column to the exact decimal text of its Number, which is
	// what the result path reads. Round-trip values a float64 cannot hold.
	for _, text := range []string{
		"0", "1", "-1", "100", "0.5", "-0.001", "9007199254740993", "12345678901234567890", "0.1234567890123456789012345678",
		"99999999999999999999999999999999999999", "123456789012345678901234567890.123456789",
	} {
		number, err := go_ora.NewNumberFromString(text)
		require.NoError(t, err, text)
		got, err := number.String()
		require.NoError(t, err, text)
		assert.Equal(t, text, got)
		assert.True(t, validJSONNumber(got), got)
		token, _, failure := encodeRemoteCell(remoteKindDecimal, got)
		require.Nil(t, failure)
		assert.Equal(t, text, token, "the cell is the exact driver text")
	}
}

func TestEncodeRemoteCell(t *testing.T) {
	token, bound, failure := encodeRemoteCell(remoteKindDecimal, nil)
	require.Nil(t, failure)
	assert.Equal(t, "null", token)
	assert.Equal(t, 4, bound)

	token, bound, failure = encodeRemoteCell(remoteKindDecimal, int64(math.MinInt64))
	require.Nil(t, failure)
	assert.Equal(t, "-9223372036854775808", token)
	assert.Equal(t, len(token)+2, bound)

	token, _, failure = encodeRemoteCell(remoteKindDecimal, []byte("1.25"))
	require.Nil(t, failure)
	assert.Equal(t, "1.25", token)

	// A short value can be replaced by the redaction marker, which is longer.
	_, bound, _ = encodeRemoteCell(remoteKindString, "ab")
	assert.Equal(t, remoteRedactedTokenBytes, bound)
	_, bound, _ = encodeRemoteCell(remoteKindDecimal, "7")
	assert.Equal(t, remoteRedactedTokenBytes, bound)

	for _, value := range []any{float64(1), float32(1), true, int(1), uint64(1), "NaN", struct{}{}} {
		_, _, failure = encodeRemoteCell(remoteKindDecimal, value)
		require.NotNil(t, failure, "%T", value)
		assert.Equal(t, remoteCodeUnsupportedValue, failure.code)
	}
	for _, value := range []any{float64(1), int64(1), true, "bad\xff", []byte("bad\xff")} {
		_, _, failure = encodeRemoteCell(remoteKindString, value)
		require.NotNil(t, failure, "%T", value)
	}
	token, _, failure = encodeRemoteCell(remoteKindString, "<&>\u2028é\x00")
	require.Nil(t, failure)
	var back string
	require.NoError(t, json.Unmarshal([]byte(token), &back))
	assert.Equal(t, "<&>\u2028é\x00", back)
	assert.NotContains(t, token, `\u003c`, "HTML characters are not escaped")
}

func TestClassifyRemoteColumn(t *testing.T) {
	col, failure := classifyRemoteColumn("ID", "NUMBER")
	require.Nil(t, failure)
	assert.Equal(t, remoteColumn{Name: "ID", VendorType: "NUMBER", LogicalType: "decimal", kind: remoteKindDecimal}, col)
	for _, driverType := range []string{"NCHAR", "VARCHAR2", "nchar"} {
		col, failure = classifyRemoteColumn("LABEL", driverType)
		require.Nil(t, failure, driverType)
		assert.Equal(t, "VARCHAR2", col.VendorType)
		assert.Equal(t, "string", col.LogicalType)
	}
	for _, unknown := range []string{"OCIClobLocator", "OCIBlobLocator", "OCIFileLocator", "LONG", "RAW", "DATE", "TIMESTAMP", "TimeStampTZ_DTY", "IBFloat", "BFloat", "XMLType", "VECTOR", "CHAR", ""} {
		_, failure = classifyRemoteColumn("C", unknown)
		require.NotNil(t, failure, unknown)
		assert.Equal(t, remoteCodeUnsupportedColumn, failure.code)
	}
	for _, name := range []string{"", strings.Repeat("c", 256), "bad\xff"} {
		_, failure = classifyRemoteColumn(name, "NUMBER")
		require.NotNil(t, failure)
		assert.Equal(t, remoteCodeInvalidColumns, failure.code)
	}
}

func TestPageEncoderQuotesCellsAsCSVAndBoundsTheResultFile(t *testing.T) {
	columns := []remoteColumn{
		{Name: "ID", VendorType: "NUMBER", LogicalType: "decimal", kind: remoteKindDecimal},
		{Name: "LABEL", VendorType: "VARCHAR2", LogicalType: "string", kind: remoteKindString},
	}
	page, failure := newPageEncoder(columns, true, testCrawlID, testTaskID, testHostname)
	require.Nil(t, failure)
	assert.True(t, page.empty())
	assert.Equal(t, page.emptyBound(), page.pageBound())

	values := [][2]any{{"1", `a,"b"`}, {"2", "line1\nline2"}, {nil, nil}, {"3.5", "  lead"}}
	var tokens [2]string
	var bounds []int
	for _, v := range values {
		bounds = bounds[:0]
		for i, col := range columns {
			token, bound, failure := encodeRemoteCell(col.kind, v[i])
			require.Nil(t, failure)
			tokens[i] = token
			bounds = append(bounds, bound)
		}
		rowBound := page.rowBound(bounds)
		before := page.fileBound(rowBound)
		require.Nil(t, page.add(tokens[:], rowBound))
		assert.Equal(t, before, page.pageBound(), "fileBound predicts the bound after the row is added")
	}
	records, err := csv.NewReader(strings.NewReader(string(page.bytes()))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 4)
	for _, rec := range records {
		require.Len(t, rec, 2)
		for _, cell := range rec {
			assert.True(t, json.Valid([]byte(cell)), "every field is exactly one JSON value: %q", cell)
		}
	}
	assert.Equal(t, int64(4), page.rows)
	page.reset()
	assert.True(t, page.empty())
	assert.Empty(t, page.bytes())
}
