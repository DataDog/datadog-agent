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

var assignedDistributedChecks []Integration
var assignedDistributedChecksMutex = sync.Mutex{}

func IsHACheck(checkID string) bool {
	assignedDistributedChecksMutex.Lock()
	defer assignedDistributedChecksMutex.Unlock()
	for _, integration := range assignedDistributedChecks {
		if integration.ID == checkID {
			return true
		}
	}
	return false
}

func SetChecks(checks []Integration) {
	assignedDistributedChecksMutex.Lock()
	defer assignedDistributedChecksMutex.Unlock()
	assignedDistributedChecks = checks
}

func ShouldRunForCheck(check check.Check) bool {
	// TODO: handle check name generically
	checkID := check.ID()
	checkName := check.String()
	log.Warnf("[ShouldRunForCheck] checkID: %s", string(checkID))

	if haagentconfig.IsEnabled() && haagentconfig.IsHAIntegration(checkName) {
		return IsHACheck(string(checkID))
	}

	return true
}
