// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancinghelpers provides helpers for the workloadbalancing component
package workloadbalancinghelpers

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// IsEnabled returns true if NDM Agent workload balancing is enabled
func IsEnabled(agentConfig model.Reader) bool {
	return agentConfig.GetBool("agent_workload_balancing.enabled")
}
