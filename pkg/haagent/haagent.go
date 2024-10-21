// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
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

func ShouldRunForCheck(check check.Check, checkLogger worker.CheckLogger) bool {
	// TODO: handle check name generically
	if IsEnabled() && check.IsHACheck() {
		isPrimary := IsPrimary()
		checkLogger.Debug("Check skipped Agent check is not run since agent is not primary")
		return isPrimary
	}

	return true
}
