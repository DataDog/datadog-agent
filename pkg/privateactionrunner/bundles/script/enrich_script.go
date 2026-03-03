// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"context"
	"strings"

	bundlesupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type EnrichScriptHandler struct{}

func NewEnrichScriptHandler() *EnrichScriptHandler {
	return &EnrichScriptHandler{}
}

type EnrichScriptInputs = bundlesupport.EnrichedActionInputs

type EnrichScriptOutputs = bundlesupport.EnrichedActionOutputs

type Scripts struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *EnrichScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[EnrichScriptInputs](task)
	if err != nil {
		return nil, err
	}

	outputs := EnrichScriptOutputs{
		Options:     []bundlesupport.LabelValue{},
		Placeholder: "Select a script",
	}

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		return nil, err
	}

	var availableScripts []bundlesupport.LabelValue
	for scriptName, scriptConf := range scriptConfig.RunPredefinedScript {
		availableScripts = append(availableScripts, bundlesupport.LabelValue{
			Label:       scriptName,
			Value:       scriptName,
			Description: strings.Join(scriptConf.Command, " "),
		})
	}

	if inputs.Search == "" {
		outputs.Options = availableScripts
		return outputs, nil
	}

	var filteredScripts []bundlesupport.LabelValue
	for _, script := range availableScripts {
		if strings.Contains(strings.ToLower(script.Label), strings.ToLower(inputs.Search)) {
			filteredScripts = append(filteredScripts, script)
		}
	}
	outputs.Options = filteredScripts
	return outputs, nil
}
