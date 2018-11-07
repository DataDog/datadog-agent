// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
)

const name = "dd_logs_config_custom_configs"

// EnvProvider implements implements the ConfigProvider interface
// It should be called once at the start of the agent.
type EnvProvider struct{}

// NewEnvProvider create a EnvProvider searching for
// configurations in env variable DD_LOGS_CONFIG_CUSTOM_CONFIGS
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// Collect get the value of env variable DD_LOGS_CONFIG_CUSTOM_CONFIGS
// and generate an integrationConfig out of it.
func (e *EnvProvider) Collect() ([]integration.Config, error) {
	customConfigs := strings.TrimSpace(config.Datadog.GetString("logs_config.custom_configs"))
	var confs []*logsConfig.LogsConfig
	var integrationConfigs []integration.Config

	if len(customConfigs) == 0 {
		return integrationConfigs, nil
	}

	err := json.Unmarshal([]byte(customConfigs), &confs)
	if err != nil {
		return integrationConfigs, err
	}

	integrationConfig := integration.Config{Provider: Env, Name: name}
	integrationConfig.LogsConfig, err = json.Marshal(confs)
	if err != nil {
		return integrationConfigs, err
	}
	integrationConfigs = append(integrationConfigs, integrationConfig)
	return integrationConfigs, nil
}

// String returns a string representation of the EnvProvider
func (e *EnvProvider) String() string {
	return Env
}

// IsUpToDate is not implemented for the env Provider as the env are not meant to change.
func (e *EnvProvider) IsUpToDate() (bool, error) {
	return false, nil
}
