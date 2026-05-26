// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/stretchr/testify/assert"
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
		require.Equal(t, defaultpaths.ConfPath, configComponentParams.defaultConfPath, "defaultConfPath values not matching")
	}
}

func TestWithCLIOverride(t *testing.T) {
	params := NewParams("test_path", WithCLIOverride("test.setting", true), WithCLIOverride("test.setting2", "test"))
	assert.Equal(t, map[string]interface{}{"test.setting": true, "test.setting2": "test"}, params.cliOverride)
}

func TestWithDefaultConfPathOverridesNewAgentParamsDefault(t *testing.T) {
	// NewAgentParams hardcodes defaultConfPath to DefaultConfPath. Verify
	// WithDefaultConfPath (applied via options) overrides that hardcoded
	// value, including the ability to set it to the empty string so the
	// config component's search-directory fallback is disabled.
	params := NewAgentParams("", WithDefaultConfPath(""))
	assert.Equal(t, "", params.defaultConfPath, "WithDefaultConfPath should override the hardcoded DefaultConfPath")

	params = NewAgentParams("", WithDefaultConfPath("/custom/path"))
	assert.Equal(t, "/custom/path", params.defaultConfPath)

	// Without the option, the platform default is preserved.
	params = NewAgentParams("")
	assert.Equal(t, defaultpaths.ConfPath, params.defaultConfPath)
}
