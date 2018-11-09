// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const name = "dd_logs_config_custom_configs"

var collectors = []func(*EnvVarProvider) ([]integration.Config, error){
	(*EnvVarProvider).collectLogsConfigs,
	(*EnvVarProvider).collectMetricsConfigs,
}

// EnvVarProvider implements implements the ConfigProvider interface
// It should be called once at the start of the agent.
type EnvVarProvider struct{}

// NewEnvVarProvider creates a EnvVarProvider searching for
// configurations in env variables.
func NewEnvVarProvider() *EnvVarProvider {
	return &EnvVarProvider{}
}

// Collect gets the value of env variables
// and generate an integrationConfig out of it.
func (e *EnvVarProvider) Collect() ([]integration.Config, error) {
	var integrationConfigs []integration.Config
	var errors []string
	var err error
	for _, collector := range collectors {
		collectorConfigs, err := collector(e)
		if err != nil {
			errors = append(errors, err.Error())
		}
		integrationConfigs = append(integrationConfigs, collectorConfigs...)
	}

	if len(errors) == 1 {
		err = fmt.Errorf("%v", errors[0])
	}
	if len(errors) > 1 {
		err = fmt.Errorf("Multiple errors: %s", strings.Join(errors, ","))
	}
	return integrationConfigs, err
}

func (e *EnvVarProvider) collectLogsConfigs() ([]integration.Config, error) {
	customConfigs := strings.TrimSpace(config.Datadog.GetString("logs_config.custom_configs"))

	if len(customConfigs) == 0 {
		return []integration.Config{}, nil
	}

	integrationConfig := integration.Config{Provider: EnvironmentVariable, Name: name}
	integrationConfig.LogsConfig = []byte(customConfigs)
	return []integration.Config{integrationConfig}, nil
}
func (e *EnvVarProvider) collectMetricsConfigs() ([]integration.Config, error) {
	return []integration.Config{}, nil
}

// String returns a string representation of the EnvVarProvider
func (e *EnvVarProvider) String() string {
	return EnvironmentVariable
}

// IsUpToDate is not implemented for the env Provider as the env are not meant to change.
func (e *EnvVarProvider) IsUpToDate() (bool, error) {
	return false, nil
}
