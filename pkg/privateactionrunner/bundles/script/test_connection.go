// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct {
}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

type TestConnectionInputs struct {
	// No inputs required for this test action
}

type TestConnectionOutputs struct {
	ConfigurationValid bool                     `json:"configurationValid"`
	AvailableScripts   map[string]ScriptDetails `json:"availableScripts"`
	Errors             []string                 `json:"errors"`
}

type ScriptDetails struct {
	Command         []string               `json:"command"`
	ParameterSchema map[string]interface{} `json:"parameterSchema,omitempty"`
	AllowedEnvVars  []string               `json:"allowedEnvVars,omitempty"`
}

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	var errors []string
	availableScripts := make(map[string]ScriptDetails)
	configurationValid := true

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		configurationValid = false
		errors = append(errors, fmt.Sprintf("Failed to parse script configuration: %v", err))
		return &TestConnectionOutputs{
			ConfigurationValid: configurationValid,
			AvailableScripts:   availableScripts,
			Errors:             errors,
		}, nil
	}

	for scriptName, scriptConf := range scriptConfig.RunPredefinedScript {
		availableScripts[scriptName] = ScriptDetails(scriptConf)
	}

	return &TestConnectionOutputs{
		ConfigurationValid: configurationValid,
		AvailableScripts:   availableScripts,
		Errors:             errors,
	}, nil
}
