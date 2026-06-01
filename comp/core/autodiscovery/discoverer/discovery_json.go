// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type discoveryService struct {
	ID    string          `json:"id"`
	Host  string          `json:"host"`
	Ports []discoveryPort `json:"ports"`
}

type discoveryPort struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type discoveredConfig struct {
	CheckName               string            `json:"check_name"`
	Name                    string            `json:"name"`
	Instances               []json.RawMessage `json:"instances"`
	InitConfig              json.RawMessage   `json:"init_config"`
	MetricConfig            json.RawMessage   `json:"metric_config"`
	LogsConfig              json.RawMessage   `json:"logs"`
	IgnoreAutodiscoveryTags bool              `json:"ignore_autodiscovery_tags"`
	CheckTagCardinality     string            `json:"check_tag_cardinality"`
}

func parseDiscoveryResult(integrationName string, resultJSON string) ([]integration.Config, error) {
	var rawConfigs []discoveredConfig
	if err := json.Unmarshal([]byte(resultJSON), &rawConfigs); err != nil {
		return nil, fmt.Errorf("could not parse discovered configs for python check %s: %w", integrationName, err)
	}

	if len(rawConfigs) == 0 {
		return nil, nil
	}

	configs := make([]integration.Config, 0, len(rawConfigs))
	for _, rawConfig := range rawConfigs {
		name := integrationName
		if rawConfig.Name != "" {
			name = rawConfig.Name
		}
		if rawConfig.CheckName != "" {
			name = rawConfig.CheckName
		}

		initConfig := rawConfig.InitConfig
		if len(initConfig) == 0 {
			initConfig = json.RawMessage("{}")
		}

		config := integration.Config{
			Name:                    name,
			InitConfig:              integration.Data(initConfig),
			MetricConfig:            integration.Data(rawConfig.MetricConfig),
			LogsConfig:              integration.Data(rawConfig.LogsConfig),
			IgnoreAutodiscoveryTags: rawConfig.IgnoreAutodiscoveryTags,
			CheckTagCardinality:     rawConfig.CheckTagCardinality,
		}
		for _, instance := range rawConfig.Instances {
			config.Instances = append(config.Instances, integration.Data(instance))
		}
		configs = append(configs, config)
	}

	return configs, nil
}
