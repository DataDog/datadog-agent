// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitFQN(t *testing.T) {
	tests := []struct {
		name           string
		fqn            string
		expectedBundle string
		expectedAction string
	}{
		{
			name:           "standard FQN",
			fqn:            "com.datadoghq.script.runScript",
			expectedBundle: "com.datadoghq.script",
			expectedAction: "runScript",
		},
		{
			name:           "short FQN",
			fqn:            "bundle.action",
			expectedBundle: "bundle",
			expectedAction: "action",
		},
		{
			name:           "single segment - no dot",
			fqn:            "action",
			expectedBundle: "",
			expectedAction: "",
		},
		{
			name:           "empty string",
			fqn:            "",
			expectedBundle: "",
			expectedAction: "",
		},
		{
			name:           "multiple dots in bundle",
			fqn:            "com.datadoghq.kubernetes.core.getResource",
			expectedBundle: "com.datadoghq.kubernetes.core",
			expectedAction: "getResource",
		},
		{
			name:           "trailing dot",
			fqn:            "bundle.",
			expectedBundle: "bundle",
			expectedAction: "",
		},
		{
			name:           "leading dot",
			fqn:            ".action",
			expectedBundle: "",
			expectedAction: "action",
		},
		{
			name:           "only dot",
			fqn:            ".",
			expectedBundle: "",
			expectedAction: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, action := SplitFQN(tt.fqn)
			assert.Equal(t, tt.expectedBundle, bundle)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

func TestIsHttpBundle(t *testing.T) {
	tests := []struct {
		name     string
		bundleId string
		expected bool
	}{
		{
			name:     "HTTP bundle",
			bundleId: "com.datadoghq.http",
			expected: true,
		},
		{
			name:     "non-HTTP bundle",
			bundleId: "com.datadoghq.script",
			expected: false,
		},
		{
			name:     "empty string",
			bundleId: "",
			expected: false,
		},
		{
			name:     "partial match - prefix",
			bundleId: "com.datadoghq.http.extra",
			expected: false,
		},
		{
			name:     "partial match - suffix",
			bundleId: "other.com.datadoghq.http",
			expected: false,
		},
		{
			name:     "case sensitive - uppercase",
			bundleId: "COM.DATADOGHQ.HTTP",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHttpBundle(tt.bundleId)
			assert.Equal(t, tt.expected, result)
		})
	}
}
