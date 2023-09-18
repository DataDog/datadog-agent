// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v3"
)

// InitConfig is used to deserialize integration init config.
type InitConfig struct {
	MinCollectionInterval int `yaml:"min_collection_interval"`
}

type QuerySamplesConfig struct {
	Enabled            bool `yaml:"enabled"`
	IncludeAllSessions bool `yaml:"include_all_sessions"`
}

type QueryMetricsConfig struct {
	Enabled            bool  `yaml:"enabled"`
	CollectionInterval int64 `yaml:"collection_interval"`
	DBRowsLimit        int   `yaml:"db_rows_limit"`
	PlanCacheRetention int   `yaml:"plan_cache_retention"`
	DisableLastActive  bool  `yaml:"disable_last_active"`
}

type SysMetricsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type TablespacesConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ProcessMemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

type SharedMemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ExecutionPlansConfig struct {
	Enabled              bool `yaml:"enabled"`
	LogUnobfuscatedPlans bool `yaml:"log_unobfuscated_plans"`
}

type AgentSQLTrace struct {
	Enabled    bool `yaml:"enabled"`
	Binds      bool `yaml:"binds"`
	Waits      bool `yaml:"waits"`
	TracedRuns int  `yaml:"traced_runs"`
}

type CustomQueryColumns struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type CustomQuery struct {
	MetricPrefix string               `yaml:"metric_prefix"`
	Pdb          string               `yaml:"pdb"`
	Query        string               `yaml:"query"`
	Columns      []CustomQueryColumns `yaml:"columns"`
	Tags         []string             `yaml:"tags"`
}

// InstanceConfig is used to deserialize integration instance config.
type InstanceConfig struct {
	Server                   string               `yaml:"server"`
	Port                     int                  `yaml:"port"`
	ServiceName              string               `yaml:"service_name"`
	Username                 string               `yaml:"username"`
	Password                 string               `yaml:"password"`
	TnsAlias                 string               `yaml:"tns_alias"`
	TnsAdmin                 string               `yaml:"tns_admin"`
	Protocol                 string               `yaml:"protocol"`
	Wallet                   string               `yaml:"wallet"`
	DBM                      bool                 `yaml:"dbm"`
	Tags                     []string             `yaml:"tags"`
	LogUnobfuscatedQueries   bool                 `yaml:"log_unobfuscated_queries"`
	ObfuscatorOptions        obfuscate.SQLConfig  `yaml:"obfuscator_options"`
	InstantClient            bool                 `yaml:"instant_client"`
	ReportedHostname         string               `yaml:"reported_hostname"`
	QuerySamples             QuerySamplesConfig   `yaml:"query_samples"`
	QueryMetrics             QueryMetricsConfig   `yaml:"query_metrics"`
	SysMetrics               SysMetricsConfig     `yaml:"sysmetrics"`
	Tablespaces              TablespacesConfig    `yaml:"tablespaces"`
	ProcessMemory            ProcessMemoryConfig  `yaml:"process_memory"`
	SharedMemory             SharedMemoryConfig   `yaml:"shared_memory"`
	ExecutionPlans           ExecutionPlansConfig `yaml:"execution_plans"`
	AgentSQLTrace            AgentSQLTrace        `yaml:"agent_sql_trace"`
	CustomQueries            []CustomQuery        `yaml:"custom_queries"`
	MetricCollectionInterval int64                `yaml:"metric_collection_interval"`
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

	// Defaults begin
	var defaultMetricCollectionInterval int64 = 60
	instance.MetricCollectionInterval = defaultMetricCollectionInterval

	instance.ObfuscatorOptions.DBMS = common.IntegrationName
	instance.ObfuscatorOptions.TableNames = true
	instance.ObfuscatorOptions.CollectCommands = true
	instance.ObfuscatorOptions.CollectComments = true

	instance.QuerySamples.Enabled = true

	instance.QueryMetrics.Enabled = true
	instance.QueryMetrics.CollectionInterval = defaultMetricCollectionInterval
	instance.QueryMetrics.DBRowsLimit = 10000
	instance.QueryMetrics.PlanCacheRetention = 15

	instance.SysMetrics.Enabled = true
	instance.Tablespaces.Enabled = true
	instance.ProcessMemory.Enabled = true
	instance.SharedMemory.Enabled = true

	instance.ExecutionPlans.Enabled = true
	// Defaults end

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
			port, err := strconv.Atoi(serverSlice[1])
			if err == nil {
				instance.Port = port
			} else {
				return nil, fmt.Errorf("Cannot extract port from server %w", err)
			}
		} else {
			instance.Port = 1521
		}
	}

	c := &CheckConfig{
		InstanceConfig: instance,
		InitConfig:     initCfg,
	}

	log.Debugf("%s@%d/%s Oracle config: %s", instance.Server, instance.Port, instance.ServiceName, c.String())

	return c, nil
}

// GetLogPrompt returns a config based prompt
func GetLogPrompt(c InstanceConfig) string {
	if c.TnsAlias != "" {
		return c.TnsAlias
	}

	var p string
	if c.Server != "" {
		p = c.Server
	}
	if c.Port != 0 {
		p = fmt.Sprintf("%s:%d", p, c.Port)
	}
	if c.ServiceName != "" {
		p = fmt.Sprintf("%s/%s", p, c.ServiceName)
	}
	return p
}
