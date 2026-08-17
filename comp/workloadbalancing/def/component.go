// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancing handles state for NDM Agent Workload Balancing.
package workloadbalancing

// team: network-device-monitoring-core

// Component is the component type.
type Component interface {
	// IsGroupActive returns whether this Agent should execute checks for the given workload
	// balancing group. A group with no assignment yet, or assigned to this Agent, is active;
	// only a group explicitly assigned to another Agent is not.
	IsGroupActive(groupID string) bool
}
