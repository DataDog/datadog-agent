// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package com_datadoghq_script provides script functionality for private action bundles.
package com_datadoghq_script //nolint:revive

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	schemaIDV1 = "script-credentials-v1" //nolint:revive
)

// ScriptBundleConfig represents the configuration for a script bundle.
type ScriptBundleConfig struct {
	SchemaID            string                               `yaml:"schemaId"` //nolint:revive
	RunPredefinedScript map[string]RunPredefinedScriptConfig `yaml:"runPredefinedScript,omitempty"`
}

// RunPredefinedScriptConfig represents the configuration for running a predefined script.
type RunPredefinedScriptConfig struct {
	Command         []string               `yaml:"command"`
	ParameterSchema map[string]interface{} `yaml:"parameterSchema,omitempty"`
}

func parseCredentials(credentials interface{}) (*ScriptBundleConfig, error) {
	tokens, ok := credentials.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected credentials to be a map[string]string, got %T", credentials)
	}
	stringConfig, ok := tokens["configFileLocation"].(string) // ResolveConnectionInfoToCredential has loaded the content of the file
	if !ok {
		return nil, fmt.Errorf("expected configFileLocation to be a string, got %T", tokens["configFileLocation"])
	}

	scriptConfig := &ScriptBundleConfig{}
	err := yaml.Unmarshal([]byte(stringConfig), scriptConfig)
	if err != nil {
		return nil, err
	}
	if scriptConfig.SchemaID != schemaIDV1 {
		return nil, fmt.Errorf("unexpected schemaId: %s, supported schemaId: %s", scriptConfig.SchemaID, schemaIDV1)
	}

	return scriptConfig, nil
}
