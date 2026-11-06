// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
	vmsgp "github.com/vmihailenco/msgpack/v5"
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
		// [dictionary of 0xffffffff strings] — the outer array declares 2 elements so it passes the
		// remaining-bytes check and the dictionary limit is what rejects the payload.
		[]byte("\x92\xdd\xff\xff\xff\xff"),
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

// TestUnmarshalMsgDictionaryBoundsAllocationsByInput checks that a few bytes of input declaring huge
// array/map counts can't make the v0.5 decoder allocate memory sized from those counts before failing.
// See https://github.com/DataDog/datadog-agent/issues/59778
func TestUnmarshalMsgDictionaryBoundsAllocationsByInput(t *testing.T) {
	const (
		huge     = 25_000_000 // the previous per-header limit
		maxAlloc = 1 << 20    // generous ceiling; each case allocated tens to hundreds of MB before the fix
	)
	// [dictionary, traces]
	dict := msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 2), huge)
	traces := msgp.AppendArrayHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 2), 0), maxV05CollectionSize)
	spans := msgp.AppendArrayHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 2), 0), 1), maxV05CollectionSize)
	// A 12-element span with dictionary refs/zeros up to Meta (9), whose map declares more entries than the
	// remaining input can hold.
	meta := msgp.AppendArrayHeader(nil, 2)
	meta = msgp.AppendString(msgp.AppendArrayHeader(meta, 1), "")                                                // dictionary: [""]
	meta = msgp.AppendArrayHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(meta, 1), 1), spanPropertyCount) // traces: [[span]]
	meta = append(meta, bytes.Repeat([]byte{0x00}, 9)...)                                                        // service..error: dictionary index 0 / zero
	meta = msgp.AppendMapHeader(meta, maxV05CollectionSize)

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"dictionary", dict},
		{"traces", traces},
		{"spans", spans},
		{"meta", meta},
		// One byte per declared entry passes the header check but not the two-values-per-entry check.
		{"meta half backed", append(bytes.Clone(meta), bytes.Repeat([]byte{0xc0}, maxV05CollectionSize)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			var tr Traces
			err := tr.UnmarshalMsgDictionary(tc.body)
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			assert.Error(t, err)
			assert.Less(t, allocated, uint64(maxAlloc), "decoding %d bytes allocated %d bytes", len(tc.body), allocated)
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
