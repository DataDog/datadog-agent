// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package idx is used to unmarshal v1.0 Trace payloads
package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vmsgp "github.com/vmihailenco/msgpack/v4"
)

func TestBuildStringTable(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		table, newZeroRef := buildStringTable([]string{})
		assert.NotNil(t, table)
		assert.Equal(t, uint32(0), newZeroRef)
		assert.Equal(t, 1, table.Len())
		assert.Equal(t, "", table.Get(0))
	})

	t.Run("empty string already at index 0", func(t *testing.T) {
		input := []string{"", "service", "name", "resource"}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		assert.Equal(t, uint32(0), newZeroRef)
		assert.Equal(t, 4, table.Len())
		assert.Equal(t, "", table.Get(0))
		assert.Equal(t, "service", table.Get(1))
		assert.Equal(t, "name", table.Get(2))
		assert.Equal(t, "resource", table.Get(3))
	})

	t.Run("empty string at index 2 - swap scenario", func(t *testing.T) {
		input := []string{"service", "name", "", "resource"}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		// The string that was at index 0 ("service") is now at index 2
		assert.Equal(t, uint32(2), newZeroRef)
		assert.Equal(t, 4, table.Len())
		// Empty string should be at index 0
		assert.Equal(t, "", table.Get(0))
		assert.Equal(t, "name", table.Get(1))
		// Original index 0 string ("service") should now be at index 2
		assert.Equal(t, "service", table.Get(2))
		assert.Equal(t, "resource", table.Get(3))
	})

	t.Run("empty string at last index - swap scenario", func(t *testing.T) {
		input := []string{"service", "name", "resource", ""}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		// The string that was at index 0 ("service") is now at index 3
		assert.Equal(t, uint32(3), newZeroRef)
		assert.Equal(t, 4, table.Len())
		// Empty string should be at index 0
		assert.Equal(t, "", table.Get(0))
		assert.Equal(t, "name", table.Get(1))
		assert.Equal(t, "resource", table.Get(2))
		// Original index 0 string ("service") should now be at index 3
		assert.Equal(t, "service", table.Get(3))
	})

	t.Run("no empty string - append scenario", func(t *testing.T) {
		input := []string{"service", "name", "resource"}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		// The string that was at index 0 ("service") is now at the end (index 3)
		assert.Equal(t, uint32(3), newZeroRef)
		assert.Equal(t, 4, table.Len())
		// Empty string should be at index 0
		assert.Equal(t, "", table.Get(0))
		assert.Equal(t, "name", table.Get(1))
		assert.Equal(t, "resource", table.Get(2))
		// Original index 0 string ("service") should be appended at the end
		assert.Equal(t, "service", table.Get(3))
	})

	t.Run("single non-empty string", func(t *testing.T) {
		input := []string{"service"}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		// The string that was at index 0 ("service") is now at index 1
		assert.Equal(t, uint32(1), newZeroRef)
		assert.Equal(t, 2, table.Len())
		// Empty string should be at index 0
		assert.Equal(t, "", table.Get(0))
		// Original string should be appended at the end
		assert.Equal(t, "service", table.Get(1))
	})

	t.Run("single empty string", func(t *testing.T) {
		input := []string{""}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		assert.Equal(t, uint32(0), newZeroRef)
		assert.Equal(t, 1, table.Len())
		assert.Equal(t, "", table.Get(0))
	})

	t.Run("multiple empty strings", func(t *testing.T) {
		// Only the first occurrence should matter
		input := []string{"service", "", "name", ""}
		table, newZeroRef := buildStringTable(input)
		assert.NotNil(t, table)
		// The string that was at index 0 ("service") is swapped with first empty string at index 1
		assert.Equal(t, uint32(1), newZeroRef)
		assert.Equal(t, 4, table.Len())
		// Empty string should be at index 0
		assert.Equal(t, "", table.Get(0))
		// Original index 0 string ("service") should now be at index 1
		assert.Equal(t, "service", table.Get(1))
		assert.Equal(t, "name", table.Get(2))
		// Second empty string remains
		assert.Equal(t, "", table.Get(3))
	})

	t.Run("preserves string references correctly", func(t *testing.T) {
		// Test that the returned newZeroRef correctly points to the moved string
		input := []string{"original-zero", "one", "two", ""}
		table, newZeroRef := buildStringTable(input)
		require.NotNil(t, table)

		// Verify the table structure
		assert.Equal(t, "", table.Get(0), "empty string should be at index 0")
		assert.Equal(t, "original-zero", table.Get(newZeroRef), "newZeroRef should point to original index 0 value")

		// Verify other strings remain accessible at their correct indices
		assert.Equal(t, "one", table.Get(1))
		assert.Equal(t, "two", table.Get(2))
	})
}

var data = [2]interface{}{
	0: []string{
		0:  "baggage",
		1:  "item",
		2:  "elasticsearch.version",
		3:  "7.0",
		4:  "my-name",
		5:  "X",
		6:  "my-service",
		7:  "my-resource",
		8:  "_dd.sampling_rate_whatever",
		9:  "value whatever",
		10: "sql",
		11: "env",
		12: "some-env",
	},
	1: [][][12]interface{}{
		{
			{
				6,
				4,
				7,
				uint64(1),
				uint64(2),
				uint64(3),
				int64(123),
				int64(456),
				1,
				map[interface{}]interface{}{
					8:  9,
					0:  1,
					2:  3,
					11: 12,
				},
				map[interface{}]float64{
					5: 1.2,
				},
				10,
			},
		},
	},
}

func TestUnmarshalMsgDictionary(t *testing.T) {
	b, err := vmsgp.Marshal(&data)
	assert.NoError(t, err)

	var tp InternalTracerPayload
	if err := tp.UnmarshalMsgDictionary(b); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(tp.Chunks))
	assert.Equal(t, 1, len(tp.Chunks[0].Spans))
	sp := tp.Chunks[0].Spans[0]
	assert.Equal(t, "my-service", sp.Service())
	assert.Equal(t, "my-name", sp.Name())
	assert.Equal(t, "my-resource", sp.Resource())
	assert.Equal(t, uint64(2), sp.SpanID())
	assert.Equal(t, uint64(3), sp.ParentID())
	assert.Equal(t, uint64(123), sp.Start())
	assert.Equal(t, uint64(456), sp.Duration())
	assert.Equal(t, true, sp.Error())
	assert.Equal(t, "sql", sp.Type())
	baggageMeta, ok := sp.GetAttributeAsString("baggage")
	assert.True(t, ok)
	assert.Equal(t, "item", baggageMeta)
	elasticsearchVersionMeta, ok := sp.GetAttributeAsString("elasticsearch.version")
	assert.True(t, ok)
	assert.Equal(t, "7.0", elasticsearchVersionMeta)
	samplingRateMeta, ok := sp.GetAttributeAsString("_dd.sampling_rate_whatever")
	assert.True(t, ok)
	assert.Equal(t, "value whatever", samplingRateMeta)
	metricX, ok := sp.GetAttributeAsFloat64("X")
	assert.True(t, ok)
	assert.Equal(t, 1.2, metricX)

	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}, tp.Chunks[0].TraceID)
	assert.Equal(t, "some-env", tp.Env())
}

func TestUnmarshalMsgDictionaryLimitsSize(t *testing.T) {
	ps := [][]byte{
		[]byte("\x9e\xdd\xff\xff\xff\xff"),
		[]byte("\x96\x97\xa40000\xa6000000\xa6000000\xa6000000\xa6000000\xa6000000\xa6000000\x96\x94\x9c\x00\x00\x0000\xd100000\xdf0000"),
		[]byte("\x90\x90\xddJ\xdc\x00A"),
		[]byte("\x9b\x91\xa20c\xdd\xe12]\b\xf60\x9a\x9a\x9a"),
	}
	for _, p := range ps {
		t.Run("", func(t *testing.T) {
			var tp InternalTracerPayload
			err := tp.UnmarshalMsgDictionary(p)
			assert.EqualError(t, err, "too long payload")
		})
	}
}
