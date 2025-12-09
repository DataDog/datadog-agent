// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_datadog_agentactions

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type DatadogAgentActions struct {
	actions map[string]types.Action
}

func NewDatadogAgentActions() *DatadogAgentActions {
	return &DatadogAgentActions{
		actions: map[string]types.Action{
			"helloWorld": NewHelloWorldHandler(),
		},
	}
}

func (h *DatadogAgentActions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
