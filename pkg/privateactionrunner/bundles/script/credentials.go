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
	SchemaId            string                               `yaml:"schemaId"`
	RunPredefinedScript map[string]RunPredefinedScriptConfig `yaml:"runPredefinedScript,omitempty"`
}

type RunPredefinedScriptConfig struct {
	Command         []string               `yaml:"command"`
	ParameterSchema map[string]interface{} `yaml:"parameterSchema,omitempty"`
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
