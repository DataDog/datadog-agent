// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAIXOsLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"7300-02-02-2419", "7.3.2.2"},
		{"7200-01-00-1842", "7.2.1.0"},
		{"7300-00-00-0000", "7.3.0.0"},
		{"7300-10-05-2501", "7.3.10.5"},
		// not enough parts
		{"7300-02", ""},
		// Version/Release too short
		{"7-02-02-2419", ""},
		// empty string
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			aixVersion, ok := ParseAIXVersion(tt.input)
			if tt.want == "" {
				assert.False(t, ok)
			} else {
				assert.True(t, ok)
				assert.Equal(t, tt.want, aixVersion.KernelVersion())
			}
		})
	}
}
