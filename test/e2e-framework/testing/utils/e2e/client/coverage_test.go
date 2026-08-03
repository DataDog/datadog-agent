// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveGoCoverWarningLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standalone warning",
			input:    goCoverDirWarning + "\n{\n\t\"result\": \"passed\"\n}\n",
			expected: "{\n\t\"result\": \"passed\"\n}\n",
		},
		{
			name:     "warning in prefixed log line",
			input:    "2026-07-17 | WARN | error: " + goCoverDirWarning + "\n{\n\t\"result\": \"passed\"\n}\n",
			expected: "{\n\t\"result\": \"passed\"\n}\n",
		},
		{
			name:     "warning without trailing newline",
			input:    "log line\n" + goCoverDirWarning,
			expected: "log line\n",
		},
		{
			name:     "unrelated output unchanged",
			input:    "log line\n{\n\t\"result\": \"passed\"\n}\n",
			expected: "log line\n{\n\t\"result\": \"passed\"\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, removeGoCoverWarningLines(test.input))
		})
	}
}
