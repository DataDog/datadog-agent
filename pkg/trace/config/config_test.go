// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/stretchr/testify/assert"
)

const (
	RunZip       = "APPSVC_RUN_ZIP"
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

func TestInAzureAppServices(t *testing.T) {
	os.Setenv(RunZip, " ")
	isLinuxAzure := inAzureAppServices()
	os.Unsetenv(RunZip)

	os.Setenv(AppLogsTrace, " ")
	isWindowsAzure := inAzureAppServices()
	os.Unsetenv(AppLogsTrace)

	isNotAzure := inAzureAppServices()

	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

func TestInitSqlObfuscationMode(t *testing.T) {
	tests := []struct {
		name     string
		conf     *AgentConfig
		expected obfuscate.ObfuscationMode
	}{
		{
			name: "obfuscate_and_normalize",
			conf: &AgentConfig{
				Features: map[string]struct{}{"sql_obfuscate_and_normalize": {}},
			},
			expected: obfuscate.ObfuscateAndNormalize,
		},
		{
			name: "obfuscate_only",
			conf: &AgentConfig{
				Features: map[string]struct{}{"sql_obfuscate_only": {}},
			},
			expected: obfuscate.ObfuscateOnly,
		},
		{
			name: "default",
			conf: &AgentConfig{
				Features: map[string]struct{}{},
			},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlObfuscateMode := initSqlObfuscationMode(tt.conf)
			assert.Equal(t, tt.expected, sqlObfuscateMode)
		})
	}
}
