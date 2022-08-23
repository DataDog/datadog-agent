// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import (
	"fmt"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

// NetflowConfig contains configuration for NetFlow collector.
type NetflowConfig struct {
	Listeners                []ListenerConfig `mapstructure:"listeners"`
	StopTimeout              int              `mapstructure:"stop_timeout"`
	AggregatorBufferSize     int              `mapstructure:"aggregator_buffer_size"`
	AggregatorFlushInterval  int              `mapstructure:"aggregator_flush_interval"`
	AggregatorFlowContextTTL int              `mapstructure:"aggregator_flow_context_ttl"`
	LogPayloads              bool             `mapstructure:"log_payloads"`
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
func ReadConfig() (*NetflowConfig, error) {
	var mainConfig NetflowConfig

	err := coreconfig.Datadog.UnmarshalKey("network_devices.netflow", &mainConfig)
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
			listenerConfig.Namespace = coreconfig.Datadog.GetString("network_devices.namespace")
		}
	}

	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}
	if mainConfig.AggregatorFlushInterval == 0 {
		mainConfig.AggregatorFlushInterval = common.DefaultAggregatorFlushInterval
	}
	if mainConfig.AggregatorFlowContextTTL == 0 {
		mainConfig.AggregatorFlowContextTTL = common.DefaultAggregatorFlowContextTTL
	}
	if mainConfig.AggregatorBufferSize == 0 {
		mainConfig.AggregatorBufferSize = common.DefaultAggregatorBufferSize
	}

	return &mainConfig, nil
}

// Addr returns the host:port address to listen on.
func (c *ListenerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
