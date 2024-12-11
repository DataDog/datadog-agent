// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

func TestSafeV4Unmarshal(t *testing.T) {
	var m runtime.MemStats
	p := []Trace{}
	var fuzzer = fuzz.NewWithSeed(1)
	for j := 0; j < 10; j++ {
		t := []*Span{
			{
				Service:    "banana",
				Name:       "name",
				Resource:   "rsc",
				Type:       "sql",
				Start:      time.Now().UnixNano(),
				SpanID:     rand.Uint64(),
				TraceID:    rand.Uint64(),
				Duration:   1e6,
				Meta:       map[string]string{"_dd.origin": "test", "env": "test"},
				MetaStruct: map[string][]byte{"_dd.origin": []byte("aaaa")},
				Metrics:    map[string]float64{"_sampling_priority_v1": 1},
			},
		}
		fuzzSpan := &Span{}
		fuzzer.Fuzz(fuzzSpan)
		t = append(t, fuzzSpan)
		p = append(p, Trace(t))
	}

	traces := Traces(p)
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	bts, err := traces.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Make the array very big with a unmarshaled field

	// Add a string field named "GARBAGE_FIELD"
	bts = append(bts, 0x85, 0xad, 0x47, 0x41, 0x52, 0x42, 0x41, 0x47, 0x45, 0x5f, 0x46, 0x49, 0x45, 0x4c, 0x44)
	// Add 50 MiB of garbage
	bts = msgp.AppendString(bts, strings.Repeat("0123456789", 5*1e6))

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))

	var resTraces Traces
	_, err = resTraces.UnmarshalMsg(bts)
	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	// running Msgsize here keeps the reference to traces array, ensuring that it's not collected by the GC
	// we can verify that the underlying array is not referenced anymore (above check)
	assert.Less(t, uint64(resTraces.Msgsize()), uint64(1e6))

	// uncomment to keep long array in RAM
	// fmt.Println(string(bts[len(bts)-2:]))
}
