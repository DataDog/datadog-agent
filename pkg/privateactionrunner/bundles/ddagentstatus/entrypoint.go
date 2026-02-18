// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_status

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// AgentStatusBundle provides actions for retrieving the core agent's status
// via the agent's IPC API.
type AgentStatusBundle struct {
	actions map[string]types.Action
}

// NewAgentStatus creates a new AgentStatusBundle with an IPC client
// for communicating with the core agent.
func NewAgentStatus(client ipc.HTTPClient) *AgentStatusBundle {
	return &AgentStatusBundle{
		actions: map[string]types.Action{
			"getStatus": NewGetCoreAgentStatusHandler(client),
		},
	}
}

// GetAction returns the action with the given name.
func (h *AgentStatusBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
