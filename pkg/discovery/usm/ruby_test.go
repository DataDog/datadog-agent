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
		name        string
		path        string
		expected    string
		shouldError bool
	}{
		{
			name:        "name is found",
			path:        "./testdata/ruby/app.rb",
			expected:    "rails_hello",
			shouldError: false,
		},
		{
			name:        "name not found",
			path:        "./testdata/ruby/app_invalid.rb",
			expected:    "",
			shouldError: true,
		},
		{
			name:        "accronym in module name",
			path:        "./testdata/ruby/app_accronym.rb",
			expected:    "http_server",
			shouldError: false,
		},
		{
			name:        "file does not exists",
			path:        "./testdata/ruby/application_does_not_exist.rb",
			expected:    "",
			shouldError: true,
		},
	}
	full, err := filepath.Abs("testdata/root")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &railsDetector{ctx: DetectionContext{
				fs:         NewSubDirFS(full),
				ContextMap: make(DetectorContextMap),
			}}

			value, err := instance.findRailsApplicationName(tt.path)

			if tt.shouldError {
				assert.Error(t, err, "did not get an error when we should have. Got service name: %s", value)
				assert.Nil(t, value, "got an non-empty result: %s", value)
				return
			}

			assert.True(t, len(tt.expected) > 0, "got an empty result")
			assert.Equal(t, tt.expected, string(value))
		})
	}
}

func TestRailsUnderscore(t *testing.T) {
	tests := []struct {
		name     string
		given    string
		expected string
	}{
		{
			name:     "one word",
			given:    "Service",
			expected: "service",
		},
		{
			name:     "accronym is preserved",
			given:    "HTTPServer",
			expected: "http_server",
		},
		{
			name:     "numbers in module name",
			given:    "HTTP2Server",
			expected: "http2_server",
		},
		{
			name:     "multiple words",
			given:    "VeryLongServiceName",
			expected: "very_long_service_name",
		},
		// NOTE: the following cases should never happen in practice
		{
			name:     "already snake case",
			given:    "service_name",
			expected: "service_name",
		},
		{
			name:     "empty name",
			given:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := railsUnderscore([]byte(tt.given))
			assert.Equal(t, tt.expected, string(got))
		})
	}
}
