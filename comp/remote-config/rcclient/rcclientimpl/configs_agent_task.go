// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclientimpl

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// parseConfigAgentTask parses an agent task config
func parseConfigAgentTask(data []byte, metadata state.Metadata) (rcclient.AgentTaskConfig, error) {
	var d rcclient.AgentTaskData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return rcclient.AgentTaskConfig{}, fmt.Errorf("Unexpected AGENT_TASK received through remote-config: %s", err)
	}

	return rcclient.AgentTaskConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}
