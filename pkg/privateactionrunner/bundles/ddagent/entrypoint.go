// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type AgentActions struct {
	actions map[string]types.Action
}

func NewAgentActions() *AgentActions {
	return &AgentActions{
		actions: map[string]types.Action{
			"agentInfo":      NewAgentInfoHandler(),
			"testConnection": NewTestConnectionHandler(),
		},
	}
}

func (h *AgentActions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
