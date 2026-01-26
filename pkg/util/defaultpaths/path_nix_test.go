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
			name:     "dogstatsd protocol log file is transformed",
			path:     dogstatsDProtocolLogFile,
			expected: "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log",
		},
		{
			name:     "pyChecksPath is transformed",
			path:     pyChecksPath,
			expected: "/opt/datadog-agent/checks.d",
		},
		{
			name:     "runPath is transformed",
			path:     "/var/run/datadog",
			expected: "/opt/datadog-agent/run",
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

func TestGettersWithCommonRoot(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	// Test without common root set
	// Note: GetRunPath() returns {InstallPath}/run for container compatibility,
	// not the FHS path /var/run/datadog. Containers mount volumes at /opt/datadog-agent/run.
	t.Run("without common root", func(t *testing.T) {
		SetCommonRoot("")
		assert.Equal(t, "/etc/datadog-agent", GetConfPath())
		assert.Equal(t, "/var/log/datadog/agent.log", GetLogFile())
		assert.Equal(t, "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log", GetDogstatsDProtocolLogFile())
		assert.Equal(t, "/opt/datadog-agent/checks.d", GetPyChecksPath())
		assert.Equal(t, "/opt/datadog-agent/run", GetRunPath())
	})

	// Test with common root set
	t.Run("with common root", func(t *testing.T) {
		SetCommonRoot("/opt/datadog-agent")
		assert.Equal(t, "/opt/datadog-agent/etc", GetConfPath())
		assert.Equal(t, "/opt/datadog-agent/logs/agent.log", GetLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/cluster-agent.log", GetDCALogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxfetch.log", GetJmxLogFile())
		// Note: filepath.Join strips trailing slashes when common root is applied
		assert.Equal(t, "/opt/datadog-agent/logs/checks", GetCheckFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxinfo", GetJMXFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log", GetDogstatsDProtocolLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log", GetStreamlogsLogFile())
		// pyChecksPath is under /opt/datadog-agent, so transformation applies
		assert.Equal(t, "/opt/datadog-agent/checks.d", GetPyChecksPath())
		// runPath transforms from /var/run/datadog to {root}/run
		assert.Equal(t, "/opt/datadog-agent/run", GetRunPath())
	})

	// Test with custom common root
	t.Run("with custom common root", func(t *testing.T) {
		SetCommonRoot("/custom/path")
		assert.Equal(t, "/custom/path/etc", GetConfPath())
		assert.Equal(t, "/custom/path/logs/agent.log", GetLogFile())
		// pyChecksPath /opt/datadog-agent/checks.d transforms to {root}/checks.d
		assert.Equal(t, "/custom/path/checks.d", GetPyChecksPath())
		// runPath /var/run/datadog transforms to {root}/run
		assert.Equal(t, "/custom/path/run", GetRunPath())
	})
}
