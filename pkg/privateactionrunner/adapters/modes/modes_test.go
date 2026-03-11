// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package modes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToStrings(t *testing.T) {
	tests := []struct {
		name     string
		modes    []Mode
		expected []string
	}{
		{
			name:     "single mode",
			modes:    []Mode{ModePull},
			expected: []string{"pull"},
		},
		{
			name:     "multiple modes",
			modes:    []Mode{ModePull, Mode("push")},
			expected: []string{"pull", "push"},
		},
		{
			name:     "empty modes",
			modes:    []Mode{},
			expected: nil,
		},
		{
			name:     "nil modes",
			modes:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToStrings(tt.modes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModeMetricTag(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		expected string
	}{
		{
			name:     "pull mode",
			mode:     ModePull,
			expected: "pull",
		},
		{
			name:     "custom mode",
			mode:     Mode("custom"),
			expected: "custom",
		},
		{
			name:     "empty mode",
			mode:     Mode(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mode.MetricTag()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModePullConstant(t *testing.T) {
	assert.Equal(t, Mode("pull"), ModePull)
}
