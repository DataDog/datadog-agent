// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "testing"

func TestTraceIDHexString(t *testing.T) {
	tests := []struct {
		name string
		id   TraceID
		want string
	}{
		{
			// dd-trace-go pprof-label path: only the lower 64 bits are
			// collected; Hi must not contribute a stray "0" prefix.
			name: "lower_only",
			id:   TraceID{Lo: 0x37e0414e1b3d14f2},
			want: "37e0414e1b3d14f2",
		},
		{
			// OTel TLS path with full 128-bit ID. dd-trace-go's 128-bit form
			// uses <unix_seconds><zero32> in the high half; the trailing
			// zeros are real bits and must be preserved by %x.
			name: "full_128bit",
			id:   TraceID{Hi: 0x6a18137300000000, Lo: 0x37e0414e1b3d14f2},
			want: "6a18137300000000" + "37e0414e1b3d14f2",
		},
		{
			// Sanity: Hi with leading zero nibbles still renders unpadded.
			name: "hi_with_leading_zero_nibble",
			id:   TraceID{Hi: 0x0a18137300000000, Lo: 0x37e0414e1b3d14f2},
			want: "a18137300000000" + "37e0414e1b3d14f2",
		},
		{
			// Hi==0: Lo's leading-zero nibbles must NOT be padded — the
			// backend's "real" 64-bit trace ID has no such zeros and
			// pattern search must match the uint64-as-hex form exactly.
			name: "lower_only_with_leading_zero",
			id:   TraceID{Lo: 0x00abcdef12345678},
			want: "abcdef12345678",
		},
		{
			// Hi!=0: Lo's leading-zero nibbles ARE part of the canonical
			// 16-byte trace ID; the result must stay 32 chars wide to
			// match APM's display form byte-for-byte.
			name: "full_128bit_lo_with_leading_zero",
			id:   TraceID{Hi: 0x6a18137300000000, Lo: 0x00abcdef12345678},
			want: "6a18137300000000" + "00abcdef12345678",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.id.HexString(); got != tc.want {
				t.Fatalf("HexString() = %q, want %q", got, tc.want)
			}
		})
	}
}
