// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package config defines the configuration options for the netflow services.
package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/pkg/snmp/utils"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

// NetflowConfig contains configuration for NetFlow collector.
type NetflowConfig struct {
	Enabled                       bool             `mapstructure:"enabled"`
	Listeners                     []ListenerConfig `mapstructure:"listeners"`
	StopTimeout                   int              `mapstructure:"stop_timeout"`
	AggregatorBufferSize          int              `mapstructure:"aggregator_buffer_size"`
	AggregatorFlushInterval       int              `mapstructure:"aggregator_flush_interval"`
	AggregatorFlowContextTTL      int              `mapstructure:"aggregator_flow_context_ttl"`
	AggregatorPortRollupThreshold int              `mapstructure:"aggregator_port_rollup_threshold"`
	AggregatorPortRollupDisabled  bool             `mapstructure:"aggregator_port_rollup_disabled"`

	// AggregatorRollupTrackerRefreshInterval is useful to speed up testing to avoid wait for 1h default
	AggregatorRollupTrackerRefreshInterval uint `mapstructure:"aggregator_rollup_tracker_refresh_interval"`

	PrometheusListenerAddress string `mapstructure:"prometheus_listener_address"` // Example `localhost:9090`
	PrometheusListenerEnabled bool   `mapstructure:"prometheus_listener_enabled"`
}

// ListenerConfig contains configuration for a single flow listener
type ListenerConfig struct {
	FlowType  common.FlowType `mapstructure:"flow_type"`
	Port      uint16          `mapstructure:"port"`
	BindHost  string          `mapstructure:"bind_host"`
	Workers   int             `mapstructure:"workers"`
	Namespace string          `mapstructure:"namespace"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(conf config.Component) (*NetflowConfig, error) {
	var mainConfig NetflowConfig

	err := conf.UnmarshalKey("network_devices.netflow", &mainConfig)
	if err != nil {
		return nil, err
	}
	for i := range mainConfig.Listeners {
		listenerConfig := &mainConfig.Listeners[i]

		flowType, err := common.GetFlowTypeByName(listenerConfig.FlowType)
		if err != nil {
			return nil, fmt.Errorf("the provided flow type `%s` is not valid (valid flow types: %v)", listenerConfig.FlowType, common.GetAllFlowTypes())
		}

		if listenerConfig.Port == 0 {
			listenerConfig.Port = flowType.DefaultPort()
			if listenerConfig.Port == 0 {
				return nil, fmt.Errorf("no default port found for `%s`, a valid port must be set", listenerConfig.FlowType)
			}
		}
		if listenerConfig.BindHost == "" {
			listenerConfig.BindHost = common.DefaultBindHost
		}
		if listenerConfig.Workers == 0 {
			listenerConfig.Workers = 1
		}
		if listenerConfig.Namespace == "" {
			listenerConfig.Namespace = conf.GetString("network_devices.namespace")
		}
		normalizedNamespace, err := utils.NormalizeNamespace(listenerConfig.Namespace)
		if err != nil {
			return nil, fmt.Errorf("invalid namespace `%s` error: %s", listenerConfig.Namespace, err)
		}
		listenerConfig.Namespace = normalizedNamespace
	}

	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}
	if mainConfig.AggregatorFlushInterval == 0 {
		mainConfig.AggregatorFlushInterval = common.DefaultAggregatorFlushInterval
	}
	if mainConfig.AggregatorFlowContextTTL == 0 {
		// Set AggregatorFlowContextTTL to AggregatorFlushInterval to keep flow context around
		// for 1 flush-interval time after a flush.
		mainConfig.AggregatorFlowContextTTL = mainConfig.AggregatorFlushInterval
	}
	if mainConfig.AggregatorBufferSize == 0 {
		mainConfig.AggregatorBufferSize = common.DefaultAggregatorBufferSize
	}
	if mainConfig.AggregatorPortRollupThreshold == 0 {
		mainConfig.AggregatorPortRollupThreshold = common.DefaultAggregatorPortRollupThreshold
	}
	if mainConfig.AggregatorRollupTrackerRefreshInterval == 0 {
		mainConfig.AggregatorRollupTrackerRefreshInterval = common.DefaultAggregatorRollupTrackerRefreshInterval
	}

	if mainConfig.PrometheusListenerAddress == "" {
		mainConfig.PrometheusListenerAddress = common.DefaultPrometheusListenerAddress
	}

	return &mainConfig, nil
}

// Addr returns the host:port address to listen on.
func (c *ListenerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
