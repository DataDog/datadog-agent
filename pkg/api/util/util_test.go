// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput bool
	}{
		{
			name:           "IPv4",
			input:          "192.168.0.1",
			expectedOutput: false,
		},
		{
			name:           "IPv6",
			input:          "2600:1f19:35d4:b900:527a:764f:e391:d369",
			expectedOutput: true,
		},
		{
			name:           "zero compressed IPv6",
			input:          "2600:1f19:35d4:b900::1",
			expectedOutput: true,
		},
		{
			name:           "IPv6 loopback",
			input:          "::1",
			expectedOutput: true,
		},
		{
			name:           "short hostname with only hexadecimal digits",
			input:          "cafe",
			expectedOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, IsIPv6(tt.input), tt.expectedOutput)
		})
	}
}
