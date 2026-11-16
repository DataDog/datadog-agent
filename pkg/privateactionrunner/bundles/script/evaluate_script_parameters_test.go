// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

const predefinedScriptConfig = `
schemaId: script-credentials-v1
runPredefinedScript:
  restart:
    command: ["systemctl", "restart", "{{ parameters.unit }}"]
    parameterSchema:
      properties:
        unit:
          type: string
      required: ["unit"]
      additionalProperties: false
      maxProperties: 1
      oneOf:
        - properties: {unit: {const: nginx}}
        - properties: {unit: {const: apache2}}
`

func loadPredefinedScript(t *testing.T, name string) RunPredefinedScriptConfig {
	t.Helper()
	config := &ScriptBundleConfig{}
	require.NoError(t, yaml.Unmarshal([]byte(predefinedScriptConfig), config))
	script, ok := config.RunPredefinedScript[name]
	require.True(t, ok)
	return script
}

func TestEvaluateScriptWithParametersAcceptsSchemaConformingInput(t *testing.T) {
	script := loadPredefinedScript(t, "restart")

	command, err := evaluateScriptWithParameters(script, map[string]interface{}{"unit": "nginx"})
	require.NoError(t, err)
	assert.Equal(t, []string{"systemctl", "restart", "nginx"}, command)
}

func TestEvaluateScriptWithParametersEnforcesRootSchemaKeywords(t *testing.T) {
	script := loadPredefinedScript(t, "restart")

	tests := []struct {
		name       string
		parameters map[string]interface{}
	}{
		{name: "additionalProperties", parameters: map[string]interface{}{"unit": "nginx", "extra": "x"}},
		{name: "oneOf", parameters: map[string]interface{}{"unit": "sshd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := evaluateScriptWithParameters(script, tt.parameters)
			require.Error(t, err)
			assert.Nil(t, command)
		})
	}
}
