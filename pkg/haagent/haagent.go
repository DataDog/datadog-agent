
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// TODO: SHOULD BE A COMPONENT WITH STATE

var runtimeRole = atomic.NewString("")

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("failover.enabled")
}

func IsPrimary() bool {
	currentRole := pkgconfigsetup.Datadog().GetString("failover.role")
	runtRole := runtimeRole.Load()
	if runtRole != "" {
		currentRole = runtRole
	}
	log.Infof("[IsPrimary] currentRole: %v", currentRole)
	return currentRole == "primary"
}

func SetRole(role string) {
	runtimeRole.Store(role)
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
	log.Warnf("[ShouldRunForCheck] check IsHACheck %s: `%t`", check.ID(), check.IsHACheck())

	if IsEnabled() && check.IsHACheck() {
		isPrimary := IsPrimary()

		// TODO: REMOVE ME
		log.Warnf("[ShouldRunForCheck] name=%s haAgentEnabled=%v role=%s isPrimary=%v",
			checkName, IsEnabled(), pkgconfigsetup.Datadog().GetString("failover.role"), isPrimary)

		if !isPrimary {
			return false
		} else {
			return true
		}
	}

	return true
}
