// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE

var runtimeRole = atomic.NewString("")
var assignedDistributedChecks []string
var assignedDistributedChecksMutex = sync.Mutex{}

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("ha_agent.enabled")
}

func IsPrimary() bool {
	currentRole := pkgconfigsetup.Datadog().GetString("ha_agent.role")
	runtRole := runtimeRole.Load()
	if runtRole != "" {
		currentRole = runtRole
	}
	// TODO: REMOVE ME
	log.Infof("[IsPrimary] currentRole: %v", currentRole)
	return currentRole == "primary"
}

func GetInitialRole() string {
	return pkgconfigsetup.Datadog().GetString("ha_agent.role")
}

func GetRole() string {
	return runtimeRole.Load()
}

func SetRole(role string) {
	runtimeRole.Store(role)
}

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
	check.InstanceConfig()
	checkName := check.String()
	idSegments := strings.Split(string(check.ID()), ":")
	checkDigest := idSegments[len(idSegments)-1]
	log.Warnf("[ShouldRunForCheck] checkID: %s", check.ID())
	log.Warnf("[ShouldRunForCheck] checkName: %s", checkName)
	log.Warnf("[ShouldRunForCheck] checkDigest: %s", checkDigest)
	log.Warnf("[ShouldRunForCheck] check inst %s: `%s`", checkName, check.InstanceConfig())
	log.Warnf("[ShouldRunForCheck] check InitConfig %s: `%s`", checkName, check.InitConfig())

	if IsEnabled() && check.IsHACheck() {
		mode := pkgconfigsetup.Datadog().GetString("ha_agent.mode")

		if mode == "distributed" {
			checkIDs := GetChecks()
			log.Warnf("[ShouldRunForCheck] checkIDs: %v", checkIDs)
			for _, validCheckId := range checkIDs {
				if !strings.Contains(validCheckId, checkName+":") {
					continue
				}
				if strings.Contains(validCheckId, ":"+checkDigest) {
					log.Warnf("[ShouldRunForCheck] found valid checkId: %v", validCheckId)
					return true
				}
			}
			log.Warnf("[ShouldRunForCheck] no valid checkId")
			return false
		} else if mode == "failover" {
			isPrimary := IsPrimary()

			// TODO: REMOVE ME
			log.Warnf("[ShouldRunForCheck] name=%s haAgentEnabled=%v role=%s isPrimary=%v",
				checkName, IsEnabled(), pkgconfigsetup.Datadog().GetString("ha_agent.role"), isPrimary)

			if !isPrimary {
				return false
			} else {
				return true
			}
		}
	}

	return true
}

func IsDistributedCheck(checkName string) bool {
	// TODO: handle check name generically
	if IsEnabled() && checkName == "snmp" {
		return true
	}
	return false
}
