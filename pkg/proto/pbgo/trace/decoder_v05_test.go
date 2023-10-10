// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	vmsgp "github.com/vmihailenco/msgpack/v4"
)

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
					8: 9,
					0: 1,
					2: 3,
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

	var traces Traces
	if err := traces.UnmarshalMsgDictionary(b); err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, traces[0][0], &Span{
		Service:  "my-service",
		Name:     "my-name",
		Resource: "my-resource",
		TraceID:  1,
		SpanID:   2,
		ParentID: 3,
		Start:    123,
		Duration: 456,
		Error:    1,
		Meta: map[string]string{
			"baggage":                    "item",
			"elasticsearch.version":      "7.0",
			"_dd.sampling_rate_whatever": "value whatever",
		},
		Metrics: map[string]float64{"X": 1.2},
		Type:    "sql",
	})
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
			var traces Traces
			err := traces.UnmarshalMsgDictionary(p)
			assert.EqualError(t, err, "too long payload")
		})
	}
}

var benchOut Traces

func BenchmarkUnmarshalMsgDictionary(b *testing.B) {
	bb, err := vmsgp.Marshal(&data)
	assert.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(bb)))
	for i := 0; i < b.N; i++ {
		assert.NoError(b, benchOut.UnmarshalMsgDictionary(bb))
	}
}
