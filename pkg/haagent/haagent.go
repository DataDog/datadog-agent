// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"sync"
)

// TODO: SHOULD BE A COMPONENT WITH STATE

var assignedIntegrations map[string]bool
var assignedIntegrationsMutex = sync.Mutex{}

func ShouldRunHAIntegrationInstance(integrationID string) bool {
	assignedIntegrationsMutex.Lock()
	defer assignedIntegrationsMutex.Unlock()
	return assignedIntegrations[integrationID]
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
