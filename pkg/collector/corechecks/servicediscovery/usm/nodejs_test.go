// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package usm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNameFromNearestPackageJSON(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "should return false when nothing found",
			path:     "./",
			expected: "",
		},
		{
			name:     "should return false when name is empty",
			path:     "./testdata/inner/index.js",
			expected: "",
		},
		{
			name:     "should return true when name is found",
			path:     "./testdata/index.js",
			expected: "my-awesome-package",
		},
	}
	full, err := filepath.Abs("testdata/root")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &nodeDetector{ctx: DetectionContext{
				fs:         NewSubDirFS(full),
				ContextMap: make(DetectorContextMap),
			}}
			value, ok := instance.findNameFromNearestPackageJSON(tt.path)
			assert.Equal(t, len(tt.expected) > 0, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}
