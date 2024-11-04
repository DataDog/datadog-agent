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
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE

var assignedDistributedChecks []string
var assignedDistributedChecksMutex = sync.Mutex{}

func GetChecks() []string {
	assignedDistributedChecksMutex.Lock()
	defer assignedDistributedChecksMutex.Unlock()
	return assignedDistributedChecks
}

func SetChecks(checks []string) {
	assignedDistributedChecksMutex.Lock()
	defer assignedDistributedChecksMutex.Unlock()
	assignedDistributedChecks = utils.CopyStrings(checks)
}

func ShouldRunForCheck(check check.Check) bool {
	// TODO: handle check name generically
	checkID := check.ID()
	checkName := check.String()
	log.Warnf("[ShouldRunForCheck] checkID: %s", string(checkID))

	if haagentconfig.IsEnabled() && haagentconfig.IsHAIntegration(checkName) {
		checkIDs := GetChecks()
		log.Warnf("[ShouldRunForCheck] checkIDs: %v", checkIDs)
		for _, validCheckId := range checkIDs {
			if validCheckId == string(checkID) {
				log.Warnf("[ShouldRunForCheck] found valid checkId: %v", validCheckId)
				return true
			}
		}
		log.Warnf("[ShouldRunForCheck] no valid checkId")
		return false
	}

	return true
}
