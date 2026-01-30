// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonRootOrPath(t *testing.T) {
	root := "/custom/root"

	tests := []struct {
		name     string
		root     string
		path     string
		expected string
	}{
		{
			name:     "empty root returns original path",
			root:     "",
			path:     "/opt/datadog-agent/logs/agent.log",
			expected: "/opt/datadog-agent/logs/agent.log",
		},
		{
			name:     "etc path is transformed",
			root:     root,
			path:     "/opt/datadog-agent/etc/datadog.yaml",
			expected: "/custom/root/etc/datadog.yaml",
		},
		{
			name:     "etc path without trailing content is transformed",
			root:     root,
			path:     "/opt/datadog-agent/etc",
			expected: "/custom/root/etc",
		},
		{
			name:     "etc conf.d path is transformed",
			root:     root,
			path:     "/opt/datadog-agent/etc/conf.d/cpu.d/conf.yaml",
			expected: "/custom/root/etc/conf.d/cpu.d/conf.yaml",
		},
		{
			name:     "logs path is transformed",
			root:     root,
			path:     "/opt/datadog-agent/logs/agent.log",
			expected: "/custom/root/logs/agent.log",
		},
		{
			name:     "logs path without trailing content is transformed",
			root:     root,
			path:     "/opt/datadog-agent/logs",
			expected: "/custom/root/logs",
		},
		{
			name:     "logs subdirectory path is transformed",
			root:     root,
			path:     "/opt/datadog-agent/logs/checks/mycheck.log",
			expected: "/custom/root/logs/checks/mycheck.log",
		},
		{
			name:     "run path is transformed",
			root:     root,
			path:     "/opt/datadog-agent/run/dsd.socket",
			expected: "/custom/root/run/dsd.socket",
		},
		{
			name:     "run path without trailing content is transformed",
			root:     root,
			path:     "/opt/datadog-agent/run",
			expected: "/custom/root/run",
		},
		{
			name:     "unknown path is not transformed",
			root:     root,
			path:     "/var/lib/datadog/something",
			expected: "/var/lib/datadog/something",
		},
		{
			name:     "dogstatsd protocol log file is transformed",
			root:     root,
			path:     dogstatsDProtocolLogFile,
			expected: "/custom/root/logs/dogstatsd_info/dogstatsd-stats.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CommonRootOrPath(tt.root, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGettersWithCommonRoot(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	// Test without common root set (should return default Darwin paths)
	t.Run("without common root", func(t *testing.T) {
		SetCommonRoot("")
		assert.Equal(t, "/opt/datadog-agent/etc", GetConfPath())
		assert.Equal(t, "/opt/datadog-agent/etc/conf.d", GetConfdPath())
		assert.Equal(t, "/opt/datadog-agent/etc/checks.d", GetAdditionalChecksPath())
		assert.Equal(t, "/opt/datadog-agent/logs/agent.log", GetLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/cluster-agent.log", GetDCALogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxfetch.log", GetJmxLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log", GetDogstatsDProtocolLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/dogstatsd.log", GetDogstatsDServiceLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/trace-agent.log", GetTraceAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log", GetStreamlogsLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/updater.log", GetUpdaterLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/security-agent.log", GetSecurityAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/process-agent.log", GetProcessAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/otel-agent.log", GetOTelAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/host-profiler.log", GetHostProfilerLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/system-probe.log", GetSystemProbeLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/checks/", GetCheckFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxinfo/", GetJMXFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/run", GetRunPath())
		assert.Equal(t, "/opt/datadog-agent/run/datadog-agent.pid", GetPidFilePath())
	})

	// Test with common root set to same path (should be no change)
	t.Run("with same common root as default", func(t *testing.T) {
		SetCommonRoot("/opt/datadog-agent")
		assert.Equal(t, "/opt/datadog-agent/etc", GetConfPath())
		assert.Equal(t, "/opt/datadog-agent/logs/agent.log", GetLogFile())
		assert.Equal(t, "/opt/datadog-agent/run", GetRunPath())
	})

	// Test with different common root
	t.Run("with custom common root", func(t *testing.T) {
		SetCommonRoot("/custom/agent")
		assert.Equal(t, "/custom/agent/etc", GetConfPath())
		assert.Equal(t, "/custom/agent/etc/conf.d", GetConfdPath())
		assert.Equal(t, "/custom/agent/etc/checks.d", GetAdditionalChecksPath())
		assert.Equal(t, "/custom/agent/logs/agent.log", GetLogFile())
		assert.Equal(t, "/custom/agent/logs/cluster-agent.log", GetDCALogFile())
		assert.Equal(t, "/custom/agent/logs/jmxfetch.log", GetJmxLogFile())
		assert.Equal(t, "/custom/agent/logs/dogstatsd_info/dogstatsd-stats.log", GetDogstatsDProtocolLogFile())
		assert.Equal(t, "/custom/agent/logs/dogstatsd.log", GetDogstatsDServiceLogFile())
		assert.Equal(t, "/custom/agent/logs/trace-agent.log", GetTraceAgentLogFile())
		assert.Equal(t, "/custom/agent/logs/streamlogs_info/streamlogs.log", GetStreamlogsLogFile())
		// Note: filepath.Join strips trailing slashes, so these don't have trailing /
		assert.Equal(t, "/custom/agent/logs/checks", GetCheckFlareDirectory())
		assert.Equal(t, "/custom/agent/logs/jmxinfo", GetJMXFlareDirectory())
		assert.Equal(t, "/custom/agent/run", GetRunPath())
		assert.Equal(t, "/custom/agent/run/datadog-agent.pid", GetPidFilePath())
	})
}

func TestAllGettersReturnNonEmpty(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	SetCommonRoot("")

	// Config paths
	assert.NotEmpty(t, GetConfPath(), "GetConfPath should not be empty")
	assert.NotEmpty(t, GetConfdPath(), "GetConfdPath should not be empty")
	assert.NotEmpty(t, GetAdditionalChecksPath(), "GetAdditionalChecksPath should not be empty")

	// Log files
	assert.NotEmpty(t, GetLogFile(), "GetLogFile should not be empty")
	assert.NotEmpty(t, GetDCALogFile(), "GetDCALogFile should not be empty")
	assert.NotEmpty(t, GetJmxLogFile(), "GetJmxLogFile should not be empty")
	assert.NotEmpty(t, GetDogstatsDProtocolLogFile(), "GetDogstatsDProtocolLogFile should not be empty")
	assert.NotEmpty(t, GetDogstatsDServiceLogFile(), "GetDogstatsDServiceLogFile should not be empty")
	assert.NotEmpty(t, GetTraceAgentLogFile(), "GetTraceAgentLogFile should not be empty")
	assert.NotEmpty(t, GetStreamlogsLogFile(), "GetStreamlogsLogFile should not be empty")
	assert.NotEmpty(t, GetUpdaterLogFile(), "GetUpdaterLogFile should not be empty")
	assert.NotEmpty(t, GetSecurityAgentLogFile(), "GetSecurityAgentLogFile should not be empty")
	assert.NotEmpty(t, GetProcessAgentLogFile(), "GetProcessAgentLogFile should not be empty")
	assert.NotEmpty(t, GetOTelAgentLogFile(), "GetOTelAgentLogFile should not be empty")
	assert.NotEmpty(t, GetHostProfilerLogFile(), "GetHostProfilerLogFile should not be empty")
	assert.NotEmpty(t, GetSystemProbeLogFile(), "GetSystemProbeLogFile should not be empty")

	// Flare directories
	assert.NotEmpty(t, GetCheckFlareDirectory(), "GetCheckFlareDirectory should not be empty")
	assert.NotEmpty(t, GetJMXFlareDirectory(), "GetJMXFlareDirectory should not be empty")

	// Run path
	assert.NotEmpty(t, GetRunPath(), "GetRunPath should not be empty")
	assert.NotEmpty(t, GetPidFilePath(), "GetPidFilePath should not be empty")

	// Note: Socket paths are empty on Darwin by default
	assert.Empty(t, GetStatsdSocket(), "GetStatsdSocket should be empty on Darwin")
	assert.Empty(t, GetReceiverSocket(), "GetReceiverSocket should be empty on Darwin")
}
