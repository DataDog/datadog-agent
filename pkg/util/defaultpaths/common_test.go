// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetAndGetCommonRoot(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	tests := []struct {
		name     string
		setRoot  string
		expected string
	}{
		{
			name:     "empty string",
			setRoot:  "",
			expected: "",
		},
		{
			name:     "default datadog path",
			setRoot:  "/opt/datadog-agent",
			expected: "/opt/datadog-agent",
		},
		{
			name:     "custom path",
			setRoot:  "/custom/install/path",
			expected: "/custom/install/path",
		},
		{
			name:     "path with trailing slash",
			setRoot:  "/opt/datadog-agent/",
			expected: "/opt/datadog-agent/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetCommonRoot(tt.setRoot)
			result := GetCommonRoot()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommonRootStateIsolation(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	// Verify changes are isolated to the test
	SetCommonRoot("/test/path/1")
	assert.Equal(t, "/test/path/1", GetCommonRoot())

	SetCommonRoot("/test/path/2")
	assert.Equal(t, "/test/path/2", GetCommonRoot())

	// Clear it
	SetCommonRoot("")
	assert.Equal(t, "", GetCommonRoot())
}
