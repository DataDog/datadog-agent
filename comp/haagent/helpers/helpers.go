// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagenthelpers provides helpers for haagent component
package haagenthelpers

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// IsEnabled returns true if HA Agent is enabled
func IsEnabled(agentConfig model.Reader) bool {
	return agentConfig.GetBool("ha_agent.enabled")
}

// GetConfigID returns config_id
func GetConfigID(agentConfig model.Reader) string {
	return agentConfig.GetString("config_id")
}
