// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

type integrationConfig struct {
	Name string `mapstructure:"name"`
}

type haAgentConfigs struct {
	enabled            bool
	group              string
	isHaIntegrationMap map[string]bool
}

func newHaAgentConfigs(agentConfig config.Component, logger log.Component) *haAgentConfigs {
	var integrationConfigs []integrationConfig
	// TODO: DO NOT EXPOSE integrations CONFIG TO USERS
	// TODO: DO NOT EXPOSE integrations CONFIG TO USERS
	// TODO: DO NOT EXPOSE integrations CONFIG TO USERS
	// TODO: DO NOT EXPOSE integrations CONFIG TO USERS

	err := agentConfig.UnmarshalKey("ha_agent.integrations", &integrationConfigs)
	if err != nil {
		logger.Errorf("Error while reading 'ha_agent.integrationConfigs' settings: %v", err)
	}
	logger.Debugf("integration configs: %v", integrationConfigs)
	integrationMap := make(map[string]bool)
	for _, intg := range integrationConfigs {
		if intg.Name == "" {
			logger.Warn("Setging 'ha_agent.integrationConfigs[].name' shouldn't be empty")
			continue
		}
		integrationMap[intg.Name] = true
	}
	return &haAgentConfigs{
		enabled:            agentConfig.GetBool("ha_agent.enabled"),
		group:              agentConfig.GetString("ha_agent.group"),
		isHaIntegrationMap: integrationMap,
	}
}
