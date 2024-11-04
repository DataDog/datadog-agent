// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/haagent/haagentconfig"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: SHOULD BE A COMPONENT WITH STATE

var assignedIntegrations map[string]bool
var assignedIntegrationsMutex = sync.Mutex{}

func IsHAIntegrationInstance(checkID string) bool {
	assignedIntegrationsMutex.Lock()
	defer assignedIntegrationsMutex.Unlock()
	return assignedIntegrations[checkID]
}

func SetIntegrationInstances(integrations []Integration) {
	integrationsMap := make(map[string]bool)
	for _, integration := range integrations {
		integrationsMap[integration.ID] = true
	}

	assignedIntegrationsMutex.Lock()
	defer assignedIntegrationsMutex.Unlock()
	assignedIntegrations = integrationsMap
}

func ShouldRunForIntegrationInstance(check check.Check) bool {
	// TODO: handle check name generically
	checkID := check.ID()
	checkName := check.String()
	log.Warnf("[ShouldRunForIntegrationInstance] checkID: %s", string(checkID))

	if haagentconfig.IsEnabled() && haagentconfig.IsHAIntegration(checkName) {
		return IsHAIntegrationInstance(string(checkID))
	}

	return true
}
