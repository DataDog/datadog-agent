// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordRoundTrip(t *testing.T) {
	r := record{contextKey: 0xdeadbeefcafe, tsUs: 1_715_000_000_123_456, value: 3.14}
	buf := appendRecord(nil, r)
	require.Len(t, buf, recordSize)
	got, err := decodeRecord(buf)
	require.NoError(t, err)
	assert.Equal(t, r, got)
}

func TestRecordFloatSpecials(t *testing.T) {
	cases := []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		math.Copysign(0, -1), // -0.0
	}
	for _, v := range cases {
		buf := appendRecord(nil, record{value: v})
		got, err := decodeRecord(buf)
		require.NoError(t, err)
		// Compare bit patterns for NaN-safe equality.
		assert.Equal(t, math.Float64bits(v), math.Float64bits(got.value))
	}
}

func TestDecodeRecordShortBuffer(t *testing.T) {
	_, err := decodeRecord(make([]byte, recordSize-1))
	assert.Error(t, err)
}

func TestReadAllRecordsPartialTrailing(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(appendRecord(nil, record{contextKey: 1, tsUs: 100, value: 1.0}))
	buf.Write(appendRecord(nil, record{contextKey: 2, tsUs: 200, value: 2.0}))
	// Write half a record to simulate a crash-truncated file.
	buf.Write(make([]byte, recordSize/2))

	recs, err := readAllRecords(&buf)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, uint64(1), recs[0].contextKey)
	assert.Equal(t, uint64(2), recs[1].contextKey)
}

func TestReadAllRecordsEmpty(t *testing.T) {
	recs, err := readAllRecords(&bytes.Buffer{})
	require.NoError(t, err)
	assert.Empty(t, recs)
}
