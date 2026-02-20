// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"fmt"
	"os/user"
	"strings"

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
	ScriptUserValid    bool                     `json:"scriptUserValid"`
	ScriptUserInfo     string                   `json:"scriptUserInfo"`
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
	scriptUserValid := true
	scriptUserInfo := ""

	scriptUserInfo, userErrors := h.validateScriptUser()
	if len(userErrors) > 0 {
		scriptUserValid = false
		errors = append(errors, userErrors...)
	}

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		configurationValid = false
		errors = append(errors, fmt.Sprintf("Failed to parse script configuration: %v", err))
		return &TestConnectionOutputs{
			ConfigurationValid: configurationValid,
			ScriptUserValid:    scriptUserValid,
			ScriptUserInfo:     scriptUserInfo,
			AvailableScripts:   availableScripts,
			Errors:             errors,
		}, nil
	}

	for scriptName, scriptConf := range scriptConfig.RunPredefinedScript {
		availableScripts[scriptName] = ScriptDetails(scriptConf)
	}

	return &TestConnectionOutputs{
		ConfigurationValid: configurationValid,
		ScriptUserValid:    scriptUserValid,
		ScriptUserInfo:     scriptUserInfo,
		AvailableScripts:   availableScripts,
		Errors:             errors,
	}, nil
}

func (h *TestConnectionHandler) validateScriptUser() (string, []string) {
	var errors []string
	var info strings.Builder

	scriptUserInfo, err := user.Lookup(ScriptUserName)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Script user '%s' not found: %v", ScriptUserName, err))
	} else {
		info.WriteString(fmt.Sprintf("Script user '%s' found (UID: %s, GID: %s)\n",
			scriptUserInfo.Username, scriptUserInfo.Uid, scriptUserInfo.Gid))
	}

	// Check if the current user can run command
	cmd, err := NewPredefinedScriptCommand(context.Background(), []string{"echo", "test"}, nil)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to build test command: %v", err))
		return info.String(), errors
	}
	_, err = cmd.CombinedOutput()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to check if the current user can use the script user: %v", err))
	} else {
		info.WriteString("Current user can use the script user.\n")
	}

	return info.String(), errors
}
