// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoolTouint64(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected uint64
	}{
		{
			name:     "true returns 1",
			input:    true,
			expected: 1,
		},
		{
			name:     "false returns 0",
			input:    false,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BoolTouint64(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHostToNetworkShort(t *testing.T) {
	tests := []struct {
		name  string
		input uint16
	}{
		{
			name:  "port 80",
			input: 80,
		},
		{
			name:  "port 443",
			input: 443,
		},
		{
			name:  "port 8080",
			input: 8080,
		},
		{
			name:  "zero",
			input: 0,
		},
		{
			name:  "max value",
			input: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HostToNetworkShort(tt.input)

			// Convert input to big endian manually to verify
			b := make([]byte, 2)
			binary.NativeEndian.PutUint16(b, tt.input)
			expected := binary.BigEndian.Uint16(b)

			assert.Equal(t, expected, result)
		})
	}
}

func TestHostToNetworkShort_ByteOrder(t *testing.T) {
	// Test that the function converts from native endian to network (big-endian) order
	original := uint16(0x1234)

	result := HostToNetworkShort(original)

	// Manually do the same conversion
	b := make([]byte, 2)
	binary.NativeEndian.PutUint16(b, original)
	expected := binary.BigEndian.Uint16(b)

	assert.Equal(t, expected, result)
}
