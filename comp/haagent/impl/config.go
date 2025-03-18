// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	helpers "github.com/DataDog/datadog-agent/comp/haagent/helpers"
)

type haAgentConfigs struct {
	enabled  bool
	configID string
}

func newHaAgentConfigs(agentConfig config.Component) *haAgentConfigs {
	return &haAgentConfigs{
		enabled:  helpers.IsEnabled(agentConfig),
		configID: helpers.GetConfigID(agentConfig),
	}
}
