// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetSocketEnvs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "file doesn't exist",
			input: "",
			expected: map[string]string{
				"DD_APM_RECEIVER_SOCKET": "/var/run/datadog/installer/apm.socket",
				"DD_DOGSTATSD_SOCKET":    "/var/run/datadog/installer/dsd.socket",
				"DD_USE_DOGSTATSD":       "true",
			},
		},
		{
			name:  "keep other envs - missing newline",
			input: "banana=true",
			expected: map[string]string{
				"DD_APM_RECEIVER_SOCKET": "/var/run/datadog/installer/apm.socket",
				"DD_DOGSTATSD_SOCKET":    "/var/run/datadog/installer/dsd.socket",
				"DD_USE_DOGSTATSD":       "true",
				"banana":                 "true",
			},
		},
		{
			name:  "keep envs - with newline",
			input: "apple=false\nat=home\n",
			expected: map[string]string{
				"DD_APM_RECEIVER_SOCKET": "/var/run/datadog/installer/apm.socket",
				"DD_DOGSTATSD_SOCKET":    "/var/run/datadog/installer/dsd.socket",
				"DD_USE_DOGSTATSD":       "true",
				"apple":                  "false",
				"at":                     "home",
			},
		},
		{
			name:  "already present",
			input: "DD_APM_RECEIVER_SOCKET=/tmp/apm.socket",
			expected: map[string]string{
				"DD_APM_RECEIVER_SOCKET": "/tmp/apm.socket",
				"DD_DOGSTATSD_SOCKET":    "/var/run/datadog/installer/dsd.socket",
				"DD_USE_DOGSTATSD":       "true",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := setSocketEnvs([]byte(tt.input))
			assert.NoError(t, err)
			envVarsCount := 0
			for _, line := range strings.Split(string(res), "\n") {
				if line == "" {
					continue
				}
				envVarsCount++
				parts := strings.SplitN(line, "=", 2)
				key := strings.TrimSpace(parts[0])
				assert.Equal(t, tt.expected[key], parts[1])
			}
			assert.Equal(t, len(tt.expected), envVarsCount)
			fmt.Println(strings.Split(string(res), "\n"), tt.expected)
		})
	}
}

func TestOldAgentPaths(t *testing.T) {
	tempDir := t.TempDir()

	agentConfigPath = filepath.Join(tempDir, "datadog.yaml")

	cleanupTestEnvironment := func() {
		os.Remove(agentConfigPath)
	}

	testCases := []struct {
		name                   string
		agentConfig            string
		expectedAPMSockPath    string
		expectedStatsdSockPath string
	}{
		{
			name:                   "Not set up",
			agentConfig:            "api_key: banana",
			expectedAPMSockPath:    apmInstallerSocket,
			expectedStatsdSockPath: statsdInstallerSocket,
		},
		{
			name: "Set up to other sockets",
			agentConfig: `
dogstatsd_socket: /banana/dsd.socket
apm_config:
  receiver_socket: /banana/apm.socket
`,
			expectedAPMSockPath:    "/banana/apm.socket",
			expectedStatsdSockPath: "/banana/dsd.socket",
		},
		{
			name: "override one socket",
			agentConfig: `
dogstatsd_socket: /banana/dsd.socket
`,
			expectedAPMSockPath:    apmInstallerSocket,
			expectedStatsdSockPath: "/banana/dsd.socket",
		},
		{
			name:                   "Fail to parse agent config",
			agentConfig:            "{}",
			expectedAPMSockPath:    apmInstallerSocket,
			expectedStatsdSockPath: statsdInstallerSocket,
		},
		{
			name:                   "Agent config does not exist",
			expectedAPMSockPath:    apmInstallerSocket,
			expectedStatsdSockPath: statsdInstallerSocket,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanupTestEnvironment()
			if tc.agentConfig != "" {
				os.WriteFile(agentConfigPath, []byte(tc.agentConfig), 0644)
			}

			apmSockPath, statsdSockPath, err := getSocketsPath()
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedAPMSockPath, apmSockPath)
			assert.Equal(t, tc.expectedStatsdSockPath, statsdSockPath)
		})
	}
}
