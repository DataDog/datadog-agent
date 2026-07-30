// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_agent

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// AgentBundle holds the actions that operate on the local datadog-agent.
type AgentBundle struct {
	actions map[string]types.Action
}

// NewAgent creates a new agent bundle. The provided IPC HTTP client is used to
// reach the local agent's authenticated command API.
func NewAgent(client ipc.HTTPClient) types.Bundle {
	return &AgentBundle{
		actions: map[string]types.Action{
			"getStatus":   NewGetStatusHandler(client),
			"getDiagnose": NewGetDiagnoseHandler(client),
			"getConfig":   NewGetConfigHandler(client),
		},
	}
}

// GetAction returns the action with the given name.
func (b *AgentBundle) GetAction(name string) types.Action {
	return b.actions[name]
}
