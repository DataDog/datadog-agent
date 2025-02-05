// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package noop

import (
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
)

type haAgentImpl struct {
}

func NewComponent() haagent.Component {
	return &haAgentImpl{}
}

func (h *haAgentImpl) Enabled() bool {
	return false
}

func (h *haAgentImpl) GetConfigID() string {
	return ""
}

func (h *haAgentImpl) IsHaIntegration(_ string) bool {
	return false
}

func (h *haAgentImpl) GetGroup() string {
	return ""
}

func (h *haAgentImpl) GetState() haagent.State {
	return haagent.State(haagent.Unknown)
}

func (h *haAgentImpl) SetLeader(_ string) {
}

// ShouldRunIntegration return true if the agent integrations should to run.
// When ha-agent is disabled, the agent behave as standalone agent (non HA) and will always run all integrations.
func (h *haAgentImpl) ShouldRunIntegration(integrationName string) bool {
	return true
}
