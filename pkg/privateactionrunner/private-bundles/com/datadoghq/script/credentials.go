package com_datadoghq_script

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	schemaIdV1 = "script-credentials-v1"
)

type ScriptBundleConfig struct {
	SchemaId            string                               `yaml:"schemaId"`
	RunPredefinedScript map[string]RunPredefinedScriptConfig `yaml:"runPredefinedScript,omitempty"`
}

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
	if scriptConfig.SchemaId != schemaIdV1 {
		return nil, fmt.Errorf("unexpected schemaId: %s, supported schemaId: %s", scriptConfig.SchemaId, schemaIdV1)
	}

	return scriptConfig, nil
}
