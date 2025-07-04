// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import (
	"fmt"

	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"

	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(conf config.Component, logger log.Component) (*NetflowConfig, error) {
	var mainConfig NetflowConfig

	err := structure.UnmarshalKey(conf, "network_devices.netflow", &mainConfig)
	if err != nil {
		return nil, err
	}
	if err = mainConfig.SetDefaults(conf.GetString("network_devices.namespace"), logger); err != nil {
		return nil, err
	}
	return &mainConfig, nil
}

// SetDefaults sets default values wherever possible, returning an error if
// any values are malformed.
func (mainConfig *NetflowConfig) SetDefaults(namespace string, logger log.Component) error {
	for i := range mainConfig.Listeners {
		listenerConfig := &mainConfig.Listeners[i]

		flowType, err := common.GetFlowTypeByName(listenerConfig.FlowType)
		if err != nil {
			return fmt.Errorf("the provided flow type `%s` is not valid (valid flow types: %v)", listenerConfig.FlowType, common.GetAllFlowTypes())
		}

		if listenerConfig.Port == 0 {
			listenerConfig.Port = flowType.DefaultPort()
			if listenerConfig.Port == 0 {
				return fmt.Errorf("no default port found for `%s`, a valid port must be set", listenerConfig.FlowType)
			}
		}
		if listenerConfig.BindHost == "" {
			listenerConfig.BindHost = common.DefaultBindHost
		}
		if listenerConfig.Workers == 0 {
			listenerConfig.Workers = 1
		}
		if listenerConfig.Namespace == "" {
			listenerConfig.Namespace = namespace
		}
		normalizedNamespace, err := utils.NormalizeNamespace(listenerConfig.Namespace)
		if err != nil {
			return fmt.Errorf("invalid namespace `%s` error: %s", listenerConfig.Namespace, err)
		}
		listenerConfig.Namespace = normalizedNamespace

		for i := range listenerConfig.Mapping {
			mapping := &listenerConfig.Mapping[i]
			fieldType, ok := common.DefaultFieldTypes[mapping.Destination]

			if ok && mapping.Type != fieldType {
				logger.Warnf("ignoring invalid mapping type %s for netflow field %s, type %s must be used for %s", mapping.Type, mapping.Destination, fieldType, mapping.Destination)
				mapping.Type = fieldType
			}
		}
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

	return nil
}
