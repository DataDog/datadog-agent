// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAdditionalHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single tag",
			input:    "workspace:peterg17",
			expected: "workspace:peterg17",
		},
		{
			name:     "comma separated",
			input:    "workspace:peterg17,env:staging",
			expected: "workspace:peterg17,env:staging",
		},
		{
			name:     "spaces within entry rejected",
			input:    "workspace:peterg17 env:staging",
			expected: "",
		},
		{
			name:     "multiple comma separated",
			input:    "workspace:peterg17,env:staging,service:web",
			expected: "workspace:peterg17,env:staging,service:web",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "uppercase normalized to lowercase",
			input:    "Workspace:PeterG17",
			expected: "workspace:peterg17",
		},
		{
			name:     "tag with dots and hyphens",
			input:    "host.name:my-host.local",
			expected: "host.name:my-host.local",
		},
		{
			name:     "tag with slashes",
			input:    "kube/namespace:default",
			expected: "kube/namespace:default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseAdditionalHeaders(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
