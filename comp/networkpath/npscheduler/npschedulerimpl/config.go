// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import "github.com/DataDog/datadog-agent/comp/core/config"

type collectorConfigs struct {
	connectionsMonitoringEnabled bool
}

func newConfig(agentConfig config.Component) *collectorConfigs {
	return &collectorConfigs{
		connectionsMonitoringEnabled: agentConfig.GetBool("network_path.connections_monitoring.enabled"),
	}
}

// networkPathCollectorEnabled checks if Network Path Collector should be enabled
// Network Path Collector is expected to be enabled if a feature depend on it.
func (c *collectorConfigs) networkPathCollectorEnabled() bool {
	return c.connectionsMonitoringEnabled
}
