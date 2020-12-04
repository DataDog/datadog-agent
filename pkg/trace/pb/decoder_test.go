// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package pb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
	vmsgp "github.com/vmihailenco/msgpack/v4"
)

func TestParseFloat64(t *testing.T) {
	assert := assert.New(t)

	data := []byte{
		0x2a,             // 42
		0xd1, 0xfb, 0x2e, // -1234
		0xcd, 0x0a, 0x9b, // 2715
		0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // float64(3.14)
	}

	reader := msgp.NewReader(bytes.NewReader(data))

	var f float64
	var err error

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(42.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(-1234.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(2715.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(3.14, f)
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

func TestDecodeMsgDictionary(t *testing.T) {
	b, err := vmsgp.Marshal(&data)
	assert.NoError(t, err)
	dc := NewMsgpReader(bytes.NewReader(b))
	defer FreeMsgpReader(dc)

	var traces Traces
	if err := traces.DecodeMsgDictionary(dc); err != nil {
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

var benchOut Traces

func BenchmarkDecodeMsgDictionary(b *testing.B) {
	bb, err := vmsgp.Marshal(&data)
	assert.NoError(b, err)
	r := bytes.NewReader(bb)
	dc := NewMsgpReader(r)
	defer FreeMsgpReader(dc)
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(bb)))
	for i := 0; i < b.N; i++ {
		r.Reset(bb)
		assert.NoError(b, benchOut.DecodeMsgDictionary(dc))
	}
}
