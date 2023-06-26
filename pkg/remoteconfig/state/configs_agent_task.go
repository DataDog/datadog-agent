// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package state

import (
	"encoding/json"
	"fmt"
)

// AgentTaskConfig is a deserialized agent task configuration file
// along with the associated metadata
type AgentTaskConfig struct {
	Config   AgentTaskData
	Metadata Metadata
}

// AgentTaskData is the content of a agent task configuration file
type AgentTaskData struct {
	TaskType string            `json:"task_type"`
	UUID     string            `json:"uuid"`
	TaskArgs map[string]string `json:"args"`
}

// ParseConfigAgentTask parses an agent task config
func ParseConfigAgentTask(data []byte, metadata Metadata) (AgentTaskConfig, error) {
	var d AgentTaskData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentTaskConfig{}, fmt.Errorf("Unexpected AGENT_TASK received through remote-config: %s", err)
	}

	return AgentTaskConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// AgentTaskConfigs returns the currently active AGENT_TASK configs
func (r *Repository) AgentTaskConfigs() map[string]AgentTaskConfig {
	typedConfigs := make(map[string]AgentTaskConfig)

	configs := r.getConfigs(ProductAgentTask)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(AgentTaskConfig)
		if !ok {
			panic("unexpected config stored as AgentTaskConfigs")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}
