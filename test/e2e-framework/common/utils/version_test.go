// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"
)

func TestParseKubernetesVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "version with SHA",
			input:    "v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027",
			expected: "v1.32.0",
		},
		{
			name:     "version without SHA",
			input:    "v1.32.0",
			expected: "v1.32.0",
		},
		{
			name:     "version without v prefix with SHA",
			input:    "1.30.0@sha256:abcdef123456",
			expected: "1.30.0",
		},
		{
			name:     "version without v prefix",
			input:    "1.30.0",
			expected: "1.30.0",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKubernetesVersion(tt.input)
			if result != tt.expected {
				t.Errorf("parseKubernetesVersion(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}
