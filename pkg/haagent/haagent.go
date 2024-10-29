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

// TODO: Should be converted into a fx component

type haAgentIntegration struct {
	Name string `json:"name"`
}

var roleStore = atomic.NewString("")

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("ha_agent.enabled")
}

func IsHACheck(integrationName string) bool {
	var integrations []haAgentIntegration
	err := pkgconfigsetup.Datadog().UnmarshalKey("ha_agent.integrations", &integrations)
	if err != nil {
		log.Errorf("Error unmarshalling integrations: %v", err)
		return false
	}
	log.Infof("integrations: %v", integrations)
	for _, integration := range integrations {
		if integration.Name == integrationName {
			return true
		}
	}
	return false
}

func IsPrimary() bool {
	return GetRole() == "primary"
}

func GetRole() string {
	role := roleStore.Load()
	if role != "" {
		return role
	}
	return pkgconfigsetup.Datadog().GetString("ha_agent.role")
}

func SetRole(role string) {
	// TODO: Need validation
	roleStore.Store(role)
}
