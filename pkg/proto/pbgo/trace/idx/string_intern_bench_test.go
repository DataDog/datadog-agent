// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"fmt"
	"testing"

	"github.com/tinylib/msgp/msgp"
)

// This file validates a single claim from the staging CPU/heap profiles:
// parseStringBytesRef materializes a Go string for every msgpack string it
// reads, then hands it to StringTable.Add, which usually finds the string
// already interned and discards the copy. The delta-heap profile attributed
// ~10GB/300s (12% of all allocation) to parseStringBytes, ~9GB of which came
// from the two span meta lines, where key/value repetition across spans in a
// payload is very high.
//
// The benchmarks below are parameterized by intern hit rate so the measured
// effect can be read against the hit rate implied by the profile (~88%).

// buildStringStream encodes total msgpack strings of which uniqueCount are
// distinct, interleaved so that repeats are spread through the stream rather
// than clustered. It returns the payload and the resulting hit rate.
func buildStringStream(total, uniqueCount int) ([]byte, float64) {
	if uniqueCount > total {
		uniqueCount = total
	}
	// Realistic shapes: short meta keys, medium meta values.
	unique := make([]string, uniqueCount)
	for i := range unique {
		if i%2 == 0 {
			unique[i] = fmt.Sprintf("http.request.header.x-correlation-id-%d", i)
		} else {
			unique[i] = fmt.Sprintf("/api/v2/resource/%d/subresource", i)
		}
	}
	var bts []byte
	for i := range total {
		bts = msgp.AppendString(bts, unique[i%uniqueCount])
	}
	return bts, 1 - float64(uniqueCount)/float64(total)
}

// BenchmarkStringTableIntern isolates the intern path: decode a stream of
// msgpack strings into a fresh StringTable, exactly as span decoding does.
func BenchmarkStringTableIntern(b *testing.B) {
	const total = 1000
	cases := []struct {
		name   string
		unique int
	}{
		{"HitRate100", 1},   // every string already interned after the first
		{"HitRate90", 100},  // near the ~88% implied by the staging profile
		{"HitRate50", 500},  // half the strings are new
		{"HitRate0", total}, // pathological: every string is new
	}
	for _, tc := range cases {
		bts, hitRate := buildStringStream(total, tc.unique)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(hitRate*100, "hit%")
			for b.Loop() {
				st := NewStringTableWithCapacity(tc.unique + 1)
				rest := bts
				for range total {
					var err error
					_, rest, err = parseStringBytesRef(st, rest)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// buildV04Span hand-encodes a v0.4 msgpack span. The idx package cannot import
// the parent trace package (that would be an import cycle), so the wire format
// is written directly.
func buildV04Span(spanIdx int) []byte {
	var b []byte
	b = msgp.AppendMapHeader(b, 12)

	b = msgp.AppendString(b, "service")
	b = msgp.AppendString(b, "my-service")
	b = msgp.AppendString(b, "name")
	b = msgp.AppendString(b, "http.request")
	b = msgp.AppendString(b, "resource")
	b = msgp.AppendString(b, "GET /api/v2/resource")
	b = msgp.AppendString(b, "type")
	b = msgp.AppendString(b, "web")
	b = msgp.AppendString(b, "trace_id")
	b = msgp.AppendUint64(b, 556677)
	b = msgp.AppendString(b, "span_id")
	b = msgp.AppendUint64(b, uint64(spanIdx+1))
	b = msgp.AppendString(b, "parent_id")
	b = msgp.AppendUint64(b, 1111)
	b = msgp.AppendString(b, "start")
	b = msgp.AppendInt64(b, 1716151234000000)
	b = msgp.AppendString(b, "duration")
	b = msgp.AppendInt64(b, 234000)
	b = msgp.AppendString(b, "error")
	b = msgp.AppendInt64(b, 0)

	// meta: 8 repeated key/value pairs plus 2 per-span-unique values, which is
	// the mix that produces the high-but-not-total hit rate seen in staging.
	b = msgp.AppendString(b, "meta")
	b = msgp.AppendMapHeader(b, 10)
	repeated := [][2]string{
		{"env", "production"},
		{"version", "1.2.3"},
		{"component", "net/http"},
		{"span.kind", "server"},
		{"_dd.p.dm", "-1"},
		{"_dd.p.tid", "672a1b2c00000000"},
		{"_dd.hostname", "i-0abc123def456789"},
		{"http.method", "GET"},
	}
	for _, kv := range repeated {
		b = msgp.AppendString(b, kv[0])
		b = msgp.AppendString(b, kv[1])
	}
	b = msgp.AppendString(b, "http.url")
	b = msgp.AppendString(b, fmt.Sprintf("https://example.com/api/v2/resource?req=%d", spanIdx))
	b = msgp.AppendString(b, "runtime-id")
	b = msgp.AppendString(b, fmt.Sprintf("8f3a2b1c-0000-0000-0000-%012d", spanIdx))

	b = msgp.AppendString(b, "metrics")
	b = msgp.AppendMapHeader(b, 2)
	b = msgp.AppendString(b, "_sampling_priority_v1")
	b = msgp.AppendFloat64(b, 2.0)
	b = msgp.AppendString(b, "_dd.top_level")
	b = msgp.AppendFloat64(b, 1.0)

	return b
}

// BenchmarkSpanUnmarshalConvertedStrings measures the end-to-end effect on the
// hot production path: decoding a payload's worth of v0.4 spans into one
// shared string table.
func BenchmarkSpanUnmarshalConvertedStrings(b *testing.B) {
	const numSpans = 100
	spans := make([][]byte, numSpans)
	totalBytes := 0
	for i := range spans {
		spans[i] = buildV04Span(i)
		totalBytes += len(spans[i])
	}

	b.ReportAllocs()
	b.SetBytes(int64(totalBytes))
	for b.Loop() {
		st := NewStringTableWithCapacity(1024)
		for i := range spans {
			s := NewInternalSpan(st, &Span{})
			cf := NewSpanConvertedFields()
			if _, err := s.UnmarshalMsgConverted(spans[i], cf); err != nil {
				b.Fatal(err)
			}
		}
	}
}
