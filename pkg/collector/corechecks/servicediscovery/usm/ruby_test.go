// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package usm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNameFromRailsApplicationRb(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "name is found",
			path:     "./testdata/application.rb",
			expected: "rails_hello",
		},
		{
			name:     "name not found",
			path:     "./testdata/application_invalid.rb",
			expected: "",
		},
	}
	full, err := filepath.Abs("testdata/root")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &railsDetector{ctx: DetectionContext{
				fs:         NewSubDirFS(full),
				contextMap: make(DetectorContextMap),
			}}
			value, ok := instance.findRailsApplicationName(tt.path)
			assert.Equal(t, len(tt.expected) > 0, ok)
			assert.Equal(t, tt.expected, railsUnderscore(value))
		})
	}
}
