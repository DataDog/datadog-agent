// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
)

// validHaIntegrations represent the list of integrations that will be considered as
// an "HA Integration", meaning it will only run on the active Agent.
// At the moment, the list of HA Integrations is hardcoded here, but we might provide
// more dynamic way to configure which integration should be considered HA Integration.
var validHaIntegrations = map[string]bool{
	// NDM integrations
	"snmp":        true,
	"cisco_aci":   true,
	"cisco_sdwan": true,

	// Other integrations
	"network_path": true,
}

type haAgentConfigs struct {
	enabled bool
	group   string
}

func newHaAgentConfigs(agentConfig config.Component) *haAgentConfigs {
	return &haAgentConfigs{
		enabled: agentConfig.GetBool("ha_agent.enabled"),
		group:   agentConfig.GetString("ha_agent.group"),
	}
}
