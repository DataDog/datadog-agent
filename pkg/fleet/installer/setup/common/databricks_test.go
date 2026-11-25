// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeWorkspaceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase and sanitize spaces",
			input:    "Example Workspace",
			expected: "example_workspace",
		},
		{
			name:     "trim quotes",
			input:    "\"example_workspace\"",
			expected: "example_workspace",
		},
		{
			name:     "dedupe underscores",
			input:    "example___workspace",
			expected: "example_workspace",
		},
		{
			name:     "remove leading and trailing underscores and other special characters except for colons and slash",
			input:    "_!:_/example_workspace!!",
			expected: ":_/example_workspace",
		},
		{
			name:     "remove leading digits",
			input:    "___123_[]workspace123",
			expected: "workspace123",
		},
		{
			name:     "allow  numbers if surrounded by alpha and non-ascii characters",
			input:    "___123_workspàce123workspacé",
			expected: "workspàce123workspacé",
		},
		{
			name:     "truncate to 200 characters",
			input:    strings.Repeat("a", 250),
			expected: strings.Repeat("a", 200),
		},
		{
			name:     "complex real-world case",
			input:    "\"___Example@Workspace#123!!___\"",
			expected: "example_workspace_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeWorkspaceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
