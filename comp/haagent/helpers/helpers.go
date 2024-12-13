// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagenthelpers provides helpers for haagent component
package haagenthelpers

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func IsEnabled(agentConfig model.Reader) bool {
	return agentConfig.GetBool("ha_agent.enabled")
}

func GetGroup(agentConfig model.Reader) string {
	return agentConfig.GetString("ha_agent.group")
}

func GetHaAgentTags(agentConfig model.Reader) []string {
	return []string{"agent_group:" + GetGroup(agentConfig)}
}
