// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const (
	schemaIdV1 = "script-credentials-v1"
)

type ScriptBundleConfig struct {
	SchemaId                      string                                         `yaml:"schemaId"`
	RunPredefinedScript           map[string]RunPredefinedScriptConfig           `yaml:"runPredefinedScript,omitempty"`
	RunPredefinedPowershellScript map[string]RunPredefinedPowershellScriptConfig `yaml:"runPredefinedPowershellScript,omitempty"`
}

type RunPredefinedScriptConfig struct {
	Command         []string               `yaml:"command"`
	ParameterSchema map[string]interface{} `yaml:"parameterSchema,omitempty"`
	AllowedEnvVars  []string               `yaml:"allowedEnvVars,omitempty"`
}

type RunPredefinedPowershellScriptConfig struct {
	// Script is an inline PowerShell script/command string
	// Users write native PowerShell syntax, e.g.:
	//   script: 'Write-Output "Hello $env:USERNAME"'
	// or multi-line:
	//   script: |
	//     $services = Get-Service
	//     $services | ConvertTo-Json
	Script string `yaml:"script,omitempty"`

	// File is a path to a .ps1 script file to execute
	// Use this for complex scripts stored in files
	File string `yaml:"file,omitempty"`

	// Arguments to pass to the script file (only used with File)
	Arguments []string `yaml:"arguments,omitempty"`

	ParameterSchema map[string]interface{} `yaml:"parameterSchema,omitempty"`
	AllowedEnvVars  []string               `yaml:"allowedEnvVars,omitempty"`
}

func parseCredentials(credentials *privateconnection.PrivateCredentials) (*ScriptBundleConfig, error) {
	tokens := credentials.AsTokenMap()
	stringConfig := tokens["configFileLocation"] // ResolveConnectionInfoToCredential has loaded the content of the file

	scriptConfig := &ScriptBundleConfig{}
	err := yaml.Unmarshal([]byte(stringConfig), scriptConfig)
	if err != nil {
		return nil, err
	}
	if scriptConfig.SchemaId != schemaIdV1 {
		return nil, fmt.Errorf("unexpected schemaId: %s, supported schemaId: %s", scriptConfig.SchemaId, schemaIdV1)
	}

	return scriptConfig, nil
}
