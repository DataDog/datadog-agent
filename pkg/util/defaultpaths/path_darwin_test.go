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
		assert.Equal(t, "/opt/datadog-agent/etc", GetDefaultConfPath())
		assert.Equal(t, "/opt/datadog-agent/etc/conf.d", GetDefaultConfdPath())
		assert.Equal(t, "/opt/datadog-agent/etc/checks.d", GetDefaultAdditionalChecksPath())
		assert.Equal(t, "/opt/datadog-agent/logs/agent.log", GetDefaultLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/cluster-agent.log", GetDefaultDCALogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxfetch.log", GetDefaultJmxLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log", GetDefaultDogstatsDProtocolLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/dogstatsd.log", GetDefaultDogstatsDServiceLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/trace-agent.log", GetDefaultTraceAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log", GetDefaultStreamlogsLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/updater.log", GetDefaultUpdaterLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/security-agent.log", GetDefaultSecurityAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/process-agent.log", GetDefaultProcessAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/otel-agent.log", GetDefaultOTelAgentLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/host-profiler.log", GetDefaultHostProfilerLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/system-probe.log", GetDefaultSystemProbeLogFile())
		assert.Equal(t, "/opt/datadog-agent/logs/checks/", GetDefaultCheckFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/logs/jmxinfo/", GetDefaultJMXFlareDirectory())
		assert.Equal(t, "/opt/datadog-agent/run", GetDefaultRunPath())
		assert.Equal(t, "/opt/datadog-agent/run/datadog-agent.pid", GetDefaultPidFilePath())
	})

	// Test with common root set to same path (should be no change)
	t.Run("with same common root as default", func(t *testing.T) {
		SetCommonRoot("/opt/datadog-agent")
		assert.Equal(t, "/opt/datadog-agent/etc", GetDefaultConfPath())
		assert.Equal(t, "/opt/datadog-agent/logs/agent.log", GetDefaultLogFile())
		assert.Equal(t, "/opt/datadog-agent/run", GetDefaultRunPath())
	})

	// Test with different common root
	t.Run("with custom common root", func(t *testing.T) {
		SetCommonRoot("/custom/agent")
		assert.Equal(t, "/custom/agent/etc", GetDefaultConfPath())
		assert.Equal(t, "/custom/agent/etc/conf.d", GetDefaultConfdPath())
		assert.Equal(t, "/custom/agent/etc/checks.d", GetDefaultAdditionalChecksPath())
		assert.Equal(t, "/custom/agent/logs/agent.log", GetDefaultLogFile())
		assert.Equal(t, "/custom/agent/logs/cluster-agent.log", GetDefaultDCALogFile())
		assert.Equal(t, "/custom/agent/logs/jmxfetch.log", GetDefaultJmxLogFile())
		assert.Equal(t, "/custom/agent/logs/dogstatsd_info/dogstatsd-stats.log", GetDefaultDogstatsDProtocolLogFile())
		assert.Equal(t, "/custom/agent/logs/dogstatsd.log", GetDefaultDogstatsDServiceLogFile())
		assert.Equal(t, "/custom/agent/logs/trace-agent.log", GetDefaultTraceAgentLogFile())
		assert.Equal(t, "/custom/agent/logs/streamlogs_info/streamlogs.log", GetDefaultStreamlogsLogFile())
		// Note: filepath.Join strips trailing slashes, so these don't have trailing /
		assert.Equal(t, "/custom/agent/logs/checks", GetDefaultCheckFlareDirectory())
		assert.Equal(t, "/custom/agent/logs/jmxinfo", GetDefaultJMXFlareDirectory())
		assert.Equal(t, "/custom/agent/run", GetDefaultRunPath())
		assert.Equal(t, "/custom/agent/run/datadog-agent.pid", GetDefaultPidFilePath())
	})
}

func TestAllGettersReturnNonEmpty(t *testing.T) {
	// Save original and restore after test
	originalRoot := commonRoot
	defer func() { commonRoot = originalRoot }()

	SetCommonRoot("")

	// Config paths
	assert.NotEmpty(t, GetDefaultConfPath(), "GetDefaultConfPath should not be empty")
	assert.NotEmpty(t, GetDefaultConfdPath(), "GetDefaultConfdPath should not be empty")
	assert.NotEmpty(t, GetDefaultAdditionalChecksPath(), "GetDefaultAdditionalChecksPath should not be empty")

	// Log files
	assert.NotEmpty(t, GetDefaultLogFile(), "GetDefaultLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultDCALogFile(), "GetDefaultDCALogFile should not be empty")
	assert.NotEmpty(t, GetDefaultJmxLogFile(), "GetDefaultJmxLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultDogstatsDProtocolLogFile(), "GetDefaultDogstatsDProtocolLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultDogstatsDServiceLogFile(), "GetDefaultDogstatsDServiceLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultTraceAgentLogFile(), "GetDefaultTraceAgentLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultStreamlogsLogFile(), "GetDefaultStreamlogsLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultUpdaterLogFile(), "GetDefaultUpdaterLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultSecurityAgentLogFile(), "GetDefaultSecurityAgentLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultProcessAgentLogFile(), "GetDefaultProcessAgentLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultOTelAgentLogFile(), "GetDefaultOTelAgentLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultHostProfilerLogFile(), "GetDefaultHostProfilerLogFile should not be empty")
	assert.NotEmpty(t, GetDefaultSystemProbeLogFile(), "GetDefaultSystemProbeLogFile should not be empty")

	// Flare directories
	assert.NotEmpty(t, GetDefaultCheckFlareDirectory(), "GetDefaultCheckFlareDirectory should not be empty")
	assert.NotEmpty(t, GetDefaultJMXFlareDirectory(), "GetDefaultJMXFlareDirectory should not be empty")

	// Run path
	assert.NotEmpty(t, GetDefaultRunPath(), "GetDefaultRunPath should not be empty")
	assert.NotEmpty(t, GetDefaultPidFilePath(), "GetDefaultPidFilePath should not be empty")

	// Note: Socket paths are empty on Darwin by default
	assert.Empty(t, GetDefaultStatsdSocket(), "GetDefaultStatsdSocket should be empty on Darwin")
	assert.Empty(t, GetDefaultReceiverSocket(), "GetDefaultReceiverSocket should be empty on Darwin")
}
