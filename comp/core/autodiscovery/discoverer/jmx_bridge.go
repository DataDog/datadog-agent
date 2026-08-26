// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build jmx

package discoverer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
)

// jmxBridge implements ConfigDiscoverer for JMX-based integrations.
// It runs JMXFetch as a one-shot subprocess in "discover" mode to probe a
// JMX endpoint, verify metrics flow, and return a config JSON string.
type jmxBridge struct{}

// NewJmxBridge returns a ConfigDiscoverer backed by JMXFetch.
func NewJmxBridge() ConfigDiscoverer {
	return &jmxBridge{}
}

func (b *jmxBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	if !isJMXIntegration(integrationName) {
		return "", fmt.Errorf("not a JMX integration: %s", integrationName)
	}
	return jmxfetch.RunDiscovery(integrationName, serviceJSON)
}

// isJMXIntegration checks whether the integration name corresponds to a known
// JMX integration.
func isJMXIntegration(name string) bool {
	_, ok := check.StandardJMXIntegrations[name]
	return ok
}
