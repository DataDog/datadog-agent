// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet128BitTraceID(t *testing.T) {
	for _, tc := range []struct {
		name      string
		span      *Span
		wantUpper uint64
		wantLower uint64
		wantErr   bool
	}{
		{
			// otel.trace_id carries the whole 128-bit ID hex encoded as 32
			// characters; only the first half is the upper 64 bits.
			name: "otel full 128-bit trace id",
			span: &Span{
				TraceID: 0x8899aabbccddeeff,
				Meta:    map[string]string{"otel.trace_id": "00112233445566778899aabbccddeeff"},
			},
			wantUpper: 0x0011223344556677,
			wantLower: 0x8899aabbccddeeff,
		},
		{
			name: "otel trace id with zero upper bits",
			span: &Span{
				TraceID: 0x8899aabbccddeeff,
				Meta:    map[string]string{"otel.trace_id": "00000000000000008899aabbccddeeff"},
			},
			wantUpper: 0,
			wantLower: 0x8899aabbccddeeff,
		},
		{
			name: "otel trace id not valid hex",
			span: &Span{
				TraceID: 0x8899aabbccddeeff,
				Meta:    map[string]string{"otel.trace_id": "zzzzzzzzzzzzzzzz8899aabbccddeeff"},
			},
			wantLower: 0x8899aabbccddeeff,
			wantErr:   true,
		},
		{
			name: "datadog _dd.p.tid",
			span: &Span{
				TraceID: 0x8899aabbccddeeff,
				Meta:    map[string]string{"_dd.p.tid": "0011223344556677"},
			},
			wantUpper: 0x0011223344556677,
			wantLower: 0x8899aabbccddeeff,
		},
		{
			name: "datadog _dd.p.tid not valid hex",
			span: &Span{
				TraceID: 0x8899aabbccddeeff,
				Meta:    map[string]string{"_dd.p.tid": "not-hex"},
			},
			wantLower: 0x8899aabbccddeeff,
			wantErr:   true,
		},
		{
			name:      "no upper bits propagated",
			span:      &Span{TraceID: 0x8899aabbccddeeff},
			wantUpper: 0,
			wantLower: 0x8899aabbccddeeff,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upper, lower, err := tc.span.Get128BitTraceID()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantUpper, upper)
			// lowerBits is always set, even on error.
			assert.Equal(t, tc.wantLower, lower)
		})
	}
}
