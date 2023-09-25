// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizer(t *testing.T) {
	tokenizer := new(tokenizer)

	type val struct {
		tokenType tokenType
		data      []byte
	}

	type testCase struct {
		path     []byte
		expected []val
	}

	testCases := []testCase{
		{
			path:     []byte("/"),
			expected: nil,
		},
		{
			path:     []byte("/abc/def"),
			expected: []val{{tokenString, []byte("abc")}, {tokenString, []byte("def")}},
		},
		{
			path:     []byte("/abc/123/def"),
			expected: []val{{tokenString, []byte("abc")}, {tokenWildcard, []byte("123")}, {tokenString, []byte("def")}},
		},
		{
			path:     []byte("/abc/def123"),
			expected: []val{{tokenString, []byte("abc")}, {tokenWildcard, []byte("def123")}},
		},
		{
			path:     []byte("/abc#def"),
			expected: []val{{tokenWildcard, []byte("abc#def")}},
		},
		{
			path:     []byte("/v5/abc"),
			expected: []val{{tokenAPIVersion, []byte("v5")}, {tokenString, []byte("abc")}},
		},
	}

	for _, tc := range testCases {
		tokenizer.Reset(tc.path)
		var got []val
		for tokenizer.Next() {
			tokenType, tokenValue := tokenizer.Value()

			// copy the data since it only remains valid for a single test case
			data := make([]byte, len(tokenValue))
			copy(data, tokenValue)

			got = append(got, val{tokenType, data})
		}

		assert.Equalf(t, tc.expected, got, "tokenization of %s should have returned %+v. got %+v", tc.path, tc.expected, got)
	}
}

func TestURLQuantizer(t *testing.T) {
	quantizer := NewURLQuantizer()

	type testCase struct {
		path     []byte
		expected []byte
	}

	testCases := []testCase{
		{
			path:     []byte("/"),
			expected: []byte("/"),
		},
		{
			path:     []byte("/abc"),
			expected: []byte("/abc"),
		},
		{
			path:     []byte("/trailing/slash/"),
			expected: []byte("/trailing/slash/"),
		},
		{
			path:     []byte("/users/1/view"),
			expected: []byte("/users/*/view"),
		},
		{
			path:     []byte("/abc/def"),
			expected: []byte("/abc/def"),
		},
		{
			path:     []byte("/abc/123/def"),
			expected: []byte("/abc/*/def"),
		},
		{
			path:     []byte("/abc/def123"),
			expected: []byte("/abc/*"),
		},
		{
			path:     []byte("/abc#def"),
			expected: []byte("/*"),
		},
		{
			path:     []byte("/v5/abc"),
			expected: []byte("/v5/abc"),
		},
		{
			path:     []byte("/latest/meta-data"),
			expected: []byte("/latest/meta-data"),
		},
		{
			path:     []byte("/health_check"),
			expected: []byte("/health_check"),
		},
		{
			path:     []byte("/abc/F05065B2-7934-4480-8500-A2C40D76F59F"),
			expected: []byte("/abc/*"),
		},
	}

	for _, tc := range testCases {
		result := quantizer.Quantize(tc.path)
		assert.Equal(t, tc.expected, result)

		// Test quantization a second time to ensure idempotency.
		// We do this to validate that bringing the quantization code to
		// the agent-side won't cause any issues for the backend, which uses a
		// similar set of heuristics. In other words, an agent payload with
		// pre-quantized endpoint arriving at the backend should be a no-op.
		result = quantizer.Quantize(result)
		assert.Equal(t, tc.expected, result)
	}
}

// The purpose of this benchmark is to ensure that the whole quantization process doesn't allocate
func BenchmarkQuantization(b *testing.B) {
	quantizer := NewURLQuantizer()

	// This should trigger the quantization since `/users/1/view` becomes
	// `/users/*/view` post-quantization (see test case above)
	path := []byte("/users/1/view")

	// We keep the index for the number so we can revert it to the original value
	numberIndex := bytes.IndexByte(path, '1')

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		quantizer.Quantize(path)

		// Restore path to it's original value
		path[numberIndex] = '1'
	}
}
