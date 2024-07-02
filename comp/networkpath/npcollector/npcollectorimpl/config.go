// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

type collectorConfigs struct {
	connectionsMonitoringEnabled bool
	workers                      int
	pathtestInputChanSize        int
	pathtestProcessingChanSize   int
	pathtestTTL                  time.Duration
	pathtestInterval             time.Duration
	flushInterval                time.Duration
}

func newConfig(agentConfig config.Component) *collectorConfigs {
	return &collectorConfigs{
		connectionsMonitoringEnabled: agentConfig.GetBool("network_path.connections_monitoring.enabled"),
		workers:                      agentConfig.GetInt("network_path.collector.workers"),
		pathtestInputChanSize:        agentConfig.GetInt("network_path.collector.input_chan_size"),
		pathtestProcessingChanSize:   agentConfig.GetInt("network_path.collector.processing_chan_size"),
		pathtestTTL:                  agentConfig.GetDuration("network_path.collector.pathtest_ttl"),
		pathtestInterval:             agentConfig.GetDuration("network_path.collector.pathtest_interval"),
		flushInterval:                agentConfig.GetDuration("network_path.collector.flush_interval"),
	}
}

// networkPathCollectorEnabled checks if Network Path Collector should be enabled
// Network Path Collector is expected to be enabled if a feature depend on it.
func (c *collectorConfigs) networkPathCollectorEnabled() bool {
	return c.connectionsMonitoringEnabled
}
