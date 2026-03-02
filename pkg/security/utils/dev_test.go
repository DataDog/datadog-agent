// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMkdev(t *testing.T) {
	tests := []struct {
		name     string
		major    uint32
		minor    uint32
		expected uint32
	}{
		{
			name:     "zero values",
			major:    0,
			minor:    0,
			expected: 0,
		},
		{
			name:     "typical disk device",
			major:    8,
			minor:    0,
			expected: 8 << 20, // (8 << 20) | 0
		},
		{
			name:     "disk partition",
			major:    8,
			minor:    1,
			expected: (8 << 20) | 1,
		},
		{
			name:     "common tty major",
			major:    4,
			minor:    64,
			expected: (4 << 20) | 64,
		},
		{
			name:     "loop device",
			major:    7,
			minor:    0,
			expected: 7 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Mkdev(tt.major, tt.minor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMkdev_Consistency(t *testing.T) {
	// Verify the kernel algorithm: (major << 20) | minor
	major := uint32(136)
	minor := uint32(42)

	result := Mkdev(major, minor)

	// Verify we can extract major and minor back
	extractedMajor := result >> 20
	extractedMinor := result & ((1 << 20) - 1)

	assert.Equal(t, major, extractedMajor)
	assert.Equal(t, minor, extractedMinor)
}
