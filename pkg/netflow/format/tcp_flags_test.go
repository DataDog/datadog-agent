// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         uint32
		expectedFlags []string
	}{
		{
			name:          "no flag",
			flags:         uint32(0),
			expectedFlags: nil,
		},
		{
			name:          "FIN",
			flags:         uint32(1),
			expectedFlags: []string{"FIN"},
		},
		{
			name:          "SYN",
			flags:         uint32(2),
			expectedFlags: []string{"SYN"},
		},
		{
			name:          "RST",
			flags:         uint32(4),
			expectedFlags: []string{"RST"},
		},
		{
			name:          "PSH",
			flags:         uint32(8),
			expectedFlags: []string{"PSH"},
		},
		{
			name:          "ACK",
			flags:         uint32(16),
			expectedFlags: []string{"ACK"},
		},
		{
			name:          "URG",
			flags:         uint32(32),
			expectedFlags: []string{"URG"},
		},
		{
			name:          "FIN SYN ACK",
			flags:         uint32(19),
			expectedFlags: []string{"FIN", "SYN", "ACK"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFlags := TCPFlags(tt.flags)
			assert.Equal(t, tt.expectedFlags, actualFlags)
		})
	}
}
