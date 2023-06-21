// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// AgentTaskConfig is a deserialized agent task configuration file
// along with the associated metadata
type AgentTaskConfig struct {
	Config   agentTaskData
	Metadata state.Metadata
}

// agentTaskData is the content of a agent task configuration file
type agentTaskData struct {
	TaskType string            `json:"task_type"`
	UUID     string            `json:"uuid"`
	TaskArgs map[string]string `json:"args"`
}

// parseConfigAgentTask parses an agent task config
func parseConfigAgentTask(data []byte, metadata state.Metadata) (AgentTaskConfig, error) {
	var d agentTaskData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentTaskConfig{}, fmt.Errorf("Unexpected AGENT_TASK received through remote-config: %s", err)
	}

	return AgentTaskConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}
