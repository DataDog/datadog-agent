// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE
// TODO: SHOULD BE A COMPONENT WITH STATE

var runtimeRole = atomic.NewString("")

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

func SetRole(role string) {
	runtimeRole.Store(role)
}

func ShouldRunForCheck(checkName string) bool {
	// TODO: handle check name generically
	if IsEnabled() && checkName == "snmp" {
		isPrimary := IsPrimary()

		// TODO: REMOVE ME
		log.Warnf("[Worker.Run] name=%s haAgentEnabled=%v role=%s isPrimary=%v",
			checkName, IsEnabled(), pkgconfigsetup.Datadog().GetString("ha_agent.role"), isPrimary)

		if !isPrimary {
			return false
		}
	}
	return true
}
