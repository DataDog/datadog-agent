// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v3"
)

// InitConfig is used to deserialize integration init config.
type InitConfig struct {
	GlobalCustomQueries   []MetricConfig `yaml:"global_custom_metrics"`
	MinCollectionInterval int            `yaml:"min_collection_interval"`
}

// InstanceConfig is used to deserialize integration instance config.
type InstanceConfig struct {
	Server                 string              `yaml:"server"`
	Port                   int                 `yaml:"port"`
	ServiceName            string              `yaml:"service_name"`
	Protocol               string              `yaml:"protocol"`
	Username               string              `yaml:"username"`
	Password               string              `yaml:"password"`
	TnsAlias               string              `yaml:"tns_alias"`
	TnsAdmin               string              `yaml:"tns_admin"`
	DBM                    bool                `yaml:"dbm"`
	Tags                   []string            `yaml:"tags"`
	LogUnobfuscatedQueries bool                `yaml:"log_unobfuscated_queries"`
	ObfuscatorOptions      obfuscate.SQLConfig `yaml:"obfuscator_options"`
	UseGodrorWithEZConnect bool                `yaml:"use_godror_with_ezconnect"`
	ReportedHostname       string              `yaml:"reported_hostname"`
	QueryMetrics           bool                `yaml:"query_metrics"`
}

// CheckConfig holds the config needed for an integration instance to run.
type CheckConfig struct {
	InitConfig
	InstanceConfig
}

// ToString returns a string representation of the CheckConfig without sensitive information.
func (c *CheckConfig) String() string {
	return fmt.Sprintf(`CheckConfig:
Server: '%s'
ServiceName: '%s'
Port: '%d'
`, c.Server, c.ServiceName, c.Port)
}

// NewCheckConfig builds a new check config.
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}
	initCfg := InitConfig{}

	instance.ObfuscatorOptions.DBMS = common.IntegrationName
	instance.ObfuscatorOptions.TableNames = true
	instance.ObfuscatorOptions.CollectCommands = true
	instance.ObfuscatorOptions.CollectComments = true

	if err := yaml.Unmarshal(rawInstance, &instance); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(rawInitConfig, &initCfg); err != nil {
		return nil, err
	}

	serverSlice := strings.Split(instance.Server, ":")
	instance.Server = serverSlice[0]
	if instance.Port == 0 {
		if len(serverSlice) > 1 {
			//instance.Port = uint64(serverSlice[1])
			instance.Port = 1522

		} else {
			instance.Port = 1523
		}
	}

	c := &CheckConfig{
		InstanceConfig: instance,
		InitConfig:     initCfg,
	}

	log.Debugf("Oracle config: %s", c.String())

	return c, nil
}
