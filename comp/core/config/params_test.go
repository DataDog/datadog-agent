// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/stretchr/testify/require"
)

func TestNewSecurityAgentParams(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		check func(params Params)
	}{
		{
			name:  "no security agent config file paths",
			input: []string{},
			check: func(params Params) {
				require.Equal(t, "", params.ConfFilePath)
				require.Equal(t, []string{}, params.securityAgentConfigFilePaths, "Security Agent Config File Paths not matching")
			},
		},
		{
			name:  "1 security agent config file path",
			input: []string{"/etc/datadog-agent/security-agent.yaml"},
			check: func(params Params) {
				require.Equal(t, "/etc/datadog-agent/security-agent.yaml", params.ConfFilePath)
				require.Equal(t, []string{"/etc/datadog-agent/security-agent.yaml"}, params.securityAgentConfigFilePaths, "Security Agent config file paths not matching")
			},
		},
		{
			name:  "more than 1 security agent config file paths",
			input: []string{"/etc/datadog-agent/security-agent.yaml", "/etc/datadog-agent/other.yaml"},
			check: func(params Params) {
				require.Equal(t, "/etc/datadog-agent/security-agent.yaml", params.ConfFilePath)
				require.Equal(t, []string{"/etc/datadog-agent/security-agent.yaml"}, params.securityAgentConfigFilePaths, "Security Agent config file paths not matching")
			},
		},
	}

	for _, test := range tests {
		configComponentParams := NewSecurityAgentParams(test.input)

		require.Equal(t, true, configComponentParams.configLoadSecurityAgent, "configLoadSecurityAgent values not matching")
		require.Equal(t, path.DefaultConfPath, configComponentParams.defaultConfPath, "defaultConfPath values not matching")
		require.Equal(t, false, configComponentParams.configMissingOK, "configMissingOK values not matching")
	}
}
