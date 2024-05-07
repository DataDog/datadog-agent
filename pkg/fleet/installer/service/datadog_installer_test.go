// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	insertedEnvs = "DD_APM_RECEIVER_SOCKET=/var/run/datadog/apm.socket\nDD_DOGSTATSD_SOCKET=/var/run/datadog/dsd.socket\nDD_USE_DOGSTATSD=true\n"
)

func TestSetEnvs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file doesn't exist",
			input:    "",
			expected: insertedEnvs,
		},
		{
			name:     "keep other envs - missing newline",
			input:    "banana=true",
			expected: "banana=true\n" + insertedEnvs,
		},
		{
			name:     "keep other envs - with newline",
			input:    "apple=false\nat=home\n",
			expected: "apple=false\nat=home\n" + insertedEnvs,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := setSocketEnvs([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(res))
		})
	}
}

func TestOldAgentPaths(t *testing.T) {
	tempDir := t.TempDir()

	agentConfigPath := filepath.Join(tempDir, "datadog.yaml")
	oldInjectPath := filepath.Join(tempDir, "old_inject")

	cleanupTestEnvironment := func() {
		os.Remove(agentConfigPath)
		os.Remove(oldInjectPath)
	}

	testCases := []struct {
		name                   string
		fileSetup              func()
		expectedAPMSockPath    string
		expectedStatsdSockPath string
	}{
		{
			name:                   "Default",
			fileSetup:              func() {},
			expectedAPMSockPath:    apmDefaultSocket,
			expectedStatsdSockPath: statsdDefaultSocket,
		},
		{
			name: "Default",
			fileSetup: func() {
				assert.Nil(t, os.Mkdir(oldInjectPath, 0755))
				assert.Nil(t, os.WriteFile(agentConfigPath, []byte("dogstatsd_socket: /banana/dsd.socket\napm_config:\n  receiver_socket: /bananaapm.socket\n"), 0644))

			},
			expectedAPMSockPath:    apmDefaultSocket,
			expectedStatsdSockPath: statsdDefaultSocket,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanupTestEnvironment()
			tc.fileSetup()

			apmSockPath, statsdSockPath := getSocketsPath(agentConfigPath, oldInjectPath)
			assert.Equal(t, tc.expectedAPMSockPath, apmSockPath)
			assert.Equal(t, tc.expectedStatsdSockPath, statsdSockPath)
		})
	}
}
