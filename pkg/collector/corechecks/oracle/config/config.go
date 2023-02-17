// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// InitConfig is used to deserialize integration init config.
type InitConfig struct {
	GlobalCustomQueries []MetricConfig `yaml:"global_custom_metrics"`
	Service             string         `yaml:"service"`
}

// InstanceConfig is used to deserialize integration instance config.
type InstanceConfig struct {
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	ServiceName string `yaml:"service_name"`
	Protocol    string `yaml:"protocol"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	TnsAlias    string `yaml:"tns_alias"`
	TnsAdmin    string `yaml:"tns_admin"`
	DBM         bool
}

// CheckConfig holds the config needed for an integration instance to run.
type CheckConfig struct {
	InitConfig
	InstanceConfig
}

// ToString returns a string representation of the CheckConfig without sensitive information.
func (c *CheckConfig) ToString() string {
	return fmt.Sprintf(`CheckConfig:
GlobalCustomQueries: '%+v'
Service: '%s'
Server: '%s'
ServiceName: '%s'
Protocol: '%s'
`, c.GlobalCustomQueries, c.Service, c.Server, c.ServiceName, c.Protocol)
}

// NewCheckConfig builds a new check config.
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}
	initCfg := InitConfig{}

	if err := yaml.Unmarshal(rawInstance, &instance); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(rawInitConfig, &initCfg); err != nil {
		return nil, err
	}

	c := &CheckConfig{
		InstanceConfig: instance,
		InitConfig:     initCfg,
	}

	return c, nil
}
