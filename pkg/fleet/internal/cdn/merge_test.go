// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeScalar(t *testing.T) {
	tests := []struct {
		name        string
		base        interface{}
		override    interface{}
		expected    interface{}
		expectedErr bool
	}{
		{
			name:     "nil base and override",
			base:     nil,
			override: nil,
			expected: nil,
		},
		{
			name:     "nil base",
			base:     nil,
			override: "override",
			expected: "override",
		},
		{
			name:     "nil override",
			base:     "base",
			override: nil,
			expected: nil,
		},
		{
			name:     "override",
			base:     "base",
			override: "override",
			expected: "override",
		},
		{
			name:        "scalar and list error",
			base:        "base",
			override:    []interface{}{"override"},
			expectedErr: true,
		},
		{
			name:        "scalar and map error",
			base:        "base",
			override:    map[string]interface{}{"key": "value"},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := merge(tt.base, tt.override)
			if tt.expectedErr {
				assert.Error(t, err, "expected an error")
			} else {
				assert.Equal(t, tt.expected, merged)
			}
		})
	}
}

func TestMergeList(t *testing.T) {
	tests := []struct {
		name        string
		base        interface{}
		override    interface{}
		expected    interface{}
		expectedErr bool
	}{
		{
			name:     "nil override",
			base:     []interface{}{"base"},
			override: nil,
			expected: nil,
		},
		{
			name:     "override",
			base:     []interface{}{"base"},
			override: []interface{}{"override"},
			expected: []interface{}{"override"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := merge(tt.base, tt.override)
			if tt.expectedErr {
				assert.Error(t, err, "expected an error")
			} else {
				assert.Equal(t, tt.expected, merged)
			}
		})
	}
}

func TestMergeMap(t *testing.T) {
	tests := []struct {
		name        string
		base        interface{}
		override    interface{}
		expected    interface{}
		expectedErr bool
	}{
		{
			name: "nil override",
			base: map[string]interface{}{
				"base": "value",
			},
			override: nil,
			expected: nil,
		},
		{
			name: "override",
			base: map[string]interface{}{
				"base": "value",
			},
			override: map[string]interface{}{
				"base": "override",
			},
			expected: map[string]interface{}{
				"base": "override",
			},
		},
		{
			name: "add key",
			base: map[string]interface{}{
				"base": "value",
			},
			override: map[string]interface{}{
				"override": "value",
			},
			expected: map[string]interface{}{
				"base":     "value",
				"override": "value",
			},
		},
		{
			name: "nested",
			base: map[string]interface{}{
				"base": map[string]interface{}{
					"key": "value",
				},
			},
			override: map[string]interface{}{
				"base": map[string]interface{}{
					"key": "override",
				},
			},
			expected: map[string]interface{}{
				"base": map[string]interface{}{
					"key": "override",
				},
			},
		},
		{
			name: "nested scalar and list error",
			base: map[string]interface{}{
				"base": map[string]interface{}{
					"key": []interface{}{"value"},
				},
			},
			override: map[string]interface{}{
				"base": map[string]interface{}{
					"key": "override",
				},
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := merge(tt.base, tt.override)
			if tt.expectedErr {
				assert.Error(t, err, "expected an error")
			} else {
				assert.Equal(t, tt.expected, merged)
			}
		})
	}
}
