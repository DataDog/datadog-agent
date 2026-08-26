// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package discoverer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// compositeDiscoverer dispatches discovery probes to the appropriate bridge
// based on the integration name. JMX integrations are handled by the JMX
// bridge; all others fall through to the Python bridge (when available).
type compositeDiscoverer struct {
	pythonBridge ConfigDiscoverer
	jmxBridge    ConfigDiscoverer
}

// NewCompositeDiscoverer returns a ConfigDiscoverer that routes JMX
// integrations to the JMX bridge and everything else to the Python bridge.
func NewCompositeDiscoverer(pythonBridge, jmxBridge ConfigDiscoverer) ConfigDiscoverer {
	return &compositeDiscoverer{
		pythonBridge: pythonBridge,
		jmxBridge:    jmxBridge,
	}
}

func (c *compositeDiscoverer) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	// Route JMX integrations to the JMX bridge
	if isJMXIntegration(integrationName) {
		if c.jmxBridge != nil {
			log.Debugf("Discovery: routing %s to JMX bridge", integrationName)
			return c.jmxBridge.DiscoverConfig(integrationName, serviceJSON)
		}
		return "", fmt.Errorf("JMX bridge not available for integration %s (agent built without JMX support)", integrationName)
	}

	// Fall through to Python bridge for non-JMX integrations
	if c.pythonBridge != nil {
		return c.pythonBridge.DiscoverConfig(integrationName, serviceJSON)
	}

	return "", fmt.Errorf("no discovery bridge available for integration %s", integrationName)
}

// isJMXIntegration checks whether the integration name corresponds to a known
// JMX integration.
func isJMXIntegration(name string) bool {
	_, ok := check.StandardJMXIntegrations[name]
	return ok
}
