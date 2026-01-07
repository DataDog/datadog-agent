// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package defaultpaths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonRootOrPath(t *testing.T) {
	root := "/opt/datadog-agent"

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty root returns original path",
			path:     "/var/log/datadog/agent.log",
			expected: "/var/log/datadog/agent.log",
		},
		{
			name:     "log path is transformed",
			path:     "/var/log/datadog/agent.log",
			expected: "/opt/datadog-agent/logs/agent.log",
		},
		{
			name:     "log subdirectory path is transformed",
			path:     "/var/log/datadog/checks/mycheck.log",
			expected: "/opt/datadog-agent/logs/checks/mycheck.log",
		},
		{
			name:     "etc path is transformed",
			path:     "/etc/datadog-agent/datadog.yaml",
			expected: "/opt/datadog-agent/etc/datadog.yaml",
		},
		{
			name:     "etc conf.d path is transformed",
			path:     "/etc/datadog-agent/conf.d/cpu.d/conf.yaml",
			expected: "/opt/datadog-agent/etc/conf.d/cpu.d/conf.yaml",
		},
		{
			name:     "run path is transformed",
			path:     "/var/run/datadog/dsd.socket",
			expected: "/opt/datadog-agent/run/dsd.socket",
		},
		{
			name:     "dogstatsd log file is transformed",
			path:     DogstatsDLogFile,
			expected: "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log",
		},
	}

	// Test with empty root - should return original path
	t.Run(tests[0].name, func(t *testing.T) {
		result := CommonRootOrPath("", tests[0].path)
		assert.Equal(t, tests[0].expected, result)
	})

	// Test remaining cases with root set
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			result := CommonRootOrPath(root, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
