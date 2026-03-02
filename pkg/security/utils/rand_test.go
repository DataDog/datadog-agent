// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "zero length",
			length: 0,
		},
		{
			name:   "small string",
			length: 5,
		},
		{
			name:   "medium string",
			length: 20,
		},
		{
			name:   "large string",
			length: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RandString(tt.length)
			assert.Len(t, result, tt.length)

			// Verify all characters are valid
			for _, r := range result {
				assert.Contains(t, string(letterRunes), string(r))
			}
		})
	}
}

func TestRandString_Uniqueness(t *testing.T) {
	// Generate multiple strings and verify they're different
	strings := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := RandString(20)
		strings[s] = true
	}
	// With 20-char strings from 52-char alphabet, collisions should be extremely rare
	assert.Greater(t, len(strings), 95, "Expected most strings to be unique")
}

func TestNewCookie(t *testing.T) {
	// Generate multiple cookies and verify they're different
	cookies := make(map[uint64]bool)
	for i := 0; i < 100; i++ {
		c := NewCookie()
		assert.NotZero(t, c)
		cookies[c] = true
	}
	// Cookies should be unique due to timestamp component
	assert.Greater(t, len(cookies), 95, "Expected most cookies to be unique")
}

func TestRandNonZeroUint64(t *testing.T) {
	// Generate multiple values and verify none are zero
	for i := 0; i < 1000; i++ {
		value := RandNonZeroUint64()
		assert.NotZero(t, value, "RandNonZeroUint64 should never return zero")
	}
}

func TestRandNonZeroUint64_Uniqueness(t *testing.T) {
	values := make(map[uint64]bool)
	for i := 0; i < 100; i++ {
		v := RandNonZeroUint64()
		values[v] = true
	}
	// With uint64 range, collisions should be extremely rare
	assert.Greater(t, len(values), 95, "Expected most values to be unique")
}
