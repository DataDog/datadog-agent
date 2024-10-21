
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent contains High Availability Agent related code
package haagent

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"go.uber.org/atomic"
)

// TODO: Should be converted into a fx component

var roleStore = atomic.NewString("")

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("failover.enabled")
}

func IsPrimary() bool {
	return GetRole() == "primary"
}

func GetRole() string {
	role := roleStore.Load()
	if role != "" {
		return role
	}
	return pkgconfigsetup.Datadog().GetString("failover.role")
}

func SetRole(role string) {
	roleStore.Store(role)
}
