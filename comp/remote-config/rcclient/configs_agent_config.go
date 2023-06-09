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

// AgentConfig is a deserialized agent configuration file
// along with the associated metadata
type AgentConfig struct {
	Config   agentConfigData
	Metadata state.Metadata
}

// ConfigContent contains the configurations set by remote-config
type ConfigContent struct {
	LogLevel string `json:"log_level"`
}

type agentConfigData struct {
	Name   string        `json:"name"`
	Config ConfigContent `json:"config"`
}

// AgentConfigOrder is a deserialized agent configuration file
// along with the associated metadata
type AgentConfigOrder struct {
	Config   agentConfigOrderData
	Metadata state.Metadata
}

type agentConfigOrderData struct {
	Order         []string `json:"order"`
	InternalOrder []string `json:"internal_order"`
}

// parseConfigAgentConfig parses an agent task config
func parseConfigAgentConfig(data []byte, metadata state.Metadata) (AgentConfig, error) {
	var d agentConfigData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("Unexpected AGENT_CONFIG received through remote-config: %s", err)
	}

	return AgentConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// parseConfigAgentConfig parses an agent task config
func parseConfigAgentConfigOrder(data []byte, metadata state.Metadata) (AgentConfigOrder, error) {
	var d agentConfigOrderData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentConfigOrder{}, fmt.Errorf("Unexpected AGENT_CONFIG received through remote-config: %s", err)
	}

	return AgentConfigOrder{
		Config:   d,
		Metadata: metadata,
	}, nil
}
