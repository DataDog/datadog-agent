// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

//nolint:revive // TODO(DBM) Fix revive linter
package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v3"
)

const defaultLoader = "core"

// InitConfig is used to deserialize integration init config.
type InitConfig struct {
	MinCollectionInterval int           `yaml:"min_collection_interval"`
	CustomQueries         []CustomQuery `yaml:"global_custom_queries"`
	UseInstantClient      bool          `yaml:"use_instant_client"`
	Service               string        `yaml:"service"`
	Loader                string        `yaml:"loader"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type QuerySamplesConfig struct {
	Enabled            bool `yaml:"enabled"`
	IncludeAllSessions bool `yaml:"include_all_sessions"`
}

type queryMetricsTrackerConfig struct {
	ContainsText []string `yaml:"contains_text"`
}

type QueryMetricsConfig struct {
	Enabled            bool                        `yaml:"enabled"`
	CollectionInterval int64                       `yaml:"collection_interval"`
	DBRowsLimit        int                         `yaml:"db_rows_limit"`
	DisableLastActive  bool                        `yaml:"disable_last_active"`
	Lookback           int64                       `yaml:"lookback"`
	Trackers           []queryMetricsTrackerConfig `yaml:"trackers"`
	MaxRunTime         int64                       `yaml:"max_run_time"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type SysMetricsConfig struct {
	Enabled bool `yaml:"enabled"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type TablespacesConfig struct {
	Enabled bool `yaml:"enabled"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type ProcessMemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

type inactiveSessionsConfig struct {
	Enabled bool `yaml:"enabled"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type SharedMemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type ExecutionPlansConfig struct {
	Enabled              bool `yaml:"enabled"`
	PlanCacheRetention   int  `yaml:"plan_cache_retention"`
	LogUnobfuscatedPlans bool `yaml:"log_unobfuscated_plans"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type AgentSQLTrace struct {
	Enabled    bool `yaml:"enabled"`
	Binds      bool `yaml:"binds"`
	Waits      bool `yaml:"waits"`
	TracedRuns int  `yaml:"traced_runs"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type CustomQueryColumns struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type CustomQuery struct {
	MetricPrefix string               `yaml:"metric_prefix"`
	Pdb          string               `yaml:"pdb"`
	Query        string               `yaml:"query"`
	Columns      []CustomQueryColumns `yaml:"columns"`
	Tags         []string             `yaml:"tags"`
}

type asmConfig struct {
	Enabled bool `yaml:"enabled"`
}

type resourceManagerConfig struct {
	Enabled bool `yaml:"enabled"`
}

type locksConfig struct {
	Enabled bool `yaml:"enabled"`
}

// InstanceConfig is used to deserialize integration instance config.
type InstanceConfig struct {
	Server                             string                 `yaml:"server"`
	Port                               int                    `yaml:"port"`
	ServiceName                        string                 `yaml:"service_name"`
	Username                           string                 `yaml:"username"`
	User                               string                 `yaml:"user"`
	Password                           string                 `yaml:"password"`
	TnsAlias                           string                 `yaml:"tns_alias"`
	TnsAdmin                           string                 `yaml:"tns_admin"`
	Protocol                           string                 `yaml:"protocol"`
	Wallet                             string                 `yaml:"wallet"`
	DBM                                bool                   `yaml:"dbm"`
	Tags                               []string               `yaml:"tags"`
	LogUnobfuscatedQueries             bool                   `yaml:"log_unobfuscated_queries"`
	ObfuscatorOptions                  obfuscate.SQLConfig    `yaml:"obfuscator_options"`
	InstantClient                      bool                   `yaml:"instant_client"`
	OracleClient                       bool                   `yaml:"oracle_client"`
	ReportedHostname                   string                 `yaml:"reported_hostname"`
	QuerySamples                       QuerySamplesConfig     `yaml:"query_samples"`
	QueryMetrics                       QueryMetricsConfig     `yaml:"query_metrics"`
	SysMetrics                         SysMetricsConfig       `yaml:"sysmetrics"`
	Tablespaces                        TablespacesConfig      `yaml:"tablespaces"`
	ProcessMemory                      ProcessMemoryConfig    `yaml:"process_memory"`
	InactiveSessions                   inactiveSessionsConfig `yaml:"inactive_sessions"`
	SharedMemory                       SharedMemoryConfig     `yaml:"shared_memory"`
	ExecutionPlans                     ExecutionPlansConfig   `yaml:"execution_plans"`
	AgentSQLTrace                      AgentSQLTrace          `yaml:"agent_sql_trace"`
	UseGlobalCustomQueries             string                 `yaml:"use_global_custom_queries"`
	CustomQueries                      []CustomQuery          `yaml:"custom_queries"`
	MetricCollectionInterval           int64                  `yaml:"metric_collection_interval"`
	DatabaseInstanceCollectionInterval uint64                 `yaml:"database_instance_collection_interval"`
	Asm                                asmConfig              `yaml:"asm"`
	ResourceManager                    resourceManagerConfig  `yaml:"resource_manager"`
	Locks                              locksConfig            `yaml:"locks"`
	OnlyCustomQueries                  bool                   `yaml:"only_custom_queries"`
	Service                            string                 `yaml:"service"`
	Loader                             string                 `yaml:"loader"`
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

// GetDefaultObfuscatorOptions return default obfuscator options
func GetDefaultObfuscatorOptions() obfuscate.SQLConfig {
	return obfuscate.SQLConfig{
		DBMS:                          common.IntegrationName,
		TableNames:                    true,
		CollectCommands:               true,
		CollectComments:               true,
		ObfuscationMode:               obfuscate.ObfuscateAndNormalize,
		RemoveSpaceBetweenParentheses: true,
		KeepNull:                      true,
		KeepTrailingSemicolon:         true,
	}
}

// NewCheckConfig builds a new check config.
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}
	initCfg := InitConfig{}

	// Defaults begin
	var defaultMetricCollectionInterval int64 = 60
	instance.MetricCollectionInterval = defaultMetricCollectionInterval

	instance.ObfuscatorOptions = GetDefaultObfuscatorOptions()

	instance.QuerySamples.Enabled = true

	instance.QueryMetrics.Enabled = true
	instance.QueryMetrics.CollectionInterval = defaultMetricCollectionInterval
	instance.QueryMetrics.DBRowsLimit = 10000
	instance.QueryMetrics.MaxRunTime = 20

	instance.ExecutionPlans.Enabled = true
	instance.ExecutionPlans.PlanCacheRetention = 15

	instance.SysMetrics.Enabled = true
	instance.Tablespaces.Enabled = true
	instance.ProcessMemory.Enabled = true
	instance.SharedMemory.Enabled = true
	instance.InactiveSessions.Enabled = true
	instance.Asm.Enabled = true
	instance.ResourceManager.Enabled = true
	instance.Locks.Enabled = true

	instance.UseGlobalCustomQueries = "true"

	instance.DatabaseInstanceCollectionInterval = 1800

	instance.Loader = defaultLoader
	initCfg.Loader = defaultLoader
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

	if instance.Username == "" {
		// For the backward compatibility with the Python integration
		if instance.User != "" {
			instance.Username = instance.User
			warnDeprecated("user", "username")
		} else {
			return nil, fmt.Errorf("`username` is not configured")
		}
	}

	/*
	 * `instant_client` is deprecated but still supported to avoid a breaking change
	 * `oracle_client` is a more appropriate naming because besides Instant Client
	 * the Agent can be used with an Oracle software home.
	 */
	if instance.InstantClient {
		instance.OracleClient = true
		warnDeprecated("instant_client", "oracle_client")
	}

	// `use_instant_client` is for backward compatibility with the old Oracle Python integration
	if initCfg.UseInstantClient {
		instance.OracleClient = true
		warnDeprecated("use_instant_client", "oracle_client in instance config")
	}

	var service string
	if instance.Service != "" {
		service = instance.Service
	} else if initCfg.Service != "" {
		service = initCfg.Service
	}
	if service != "" {
		instance.Tags = append(instance.Tags, fmt.Sprintf("service:%s", service))
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
	return fmt.Sprintf("%s>", GetConnectData(c))
}

// GetConnectData returns the connection configuration
func GetConnectData(c InstanceConfig) string {
	if c.TnsAlias != "" {
		return c.TnsAlias
	}

	var p string
	if c.Server != "" {
		p = c.Server
		if c.ReportedHostname != "" {
			p = fmt.Sprintf("%s[%s]", p, c.ReportedHostname)
		}
	}
	if c.Port != 0 {
		p = fmt.Sprintf("%s:%d", p, c.Port)
	}
	if c.ServiceName != "" {
		p = fmt.Sprintf("%s/%s", p, c.ServiceName)
	}
	return p
}

func warnDeprecated(old string, new string) {
	log.Warnf("The config parameter %s is deprecated and will be removed in future versions. Please use %s instead.", old, new)
}
