// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancing holds the per-group active/standby state used to hand a
// network device from one Agent to another without restarting either Agent.
package workloadbalancing

// team: network-device-monitoring-core

// Component is the component type.
type Component interface {
	// Enabled returns true if agent_workload_balancing.enabled is set to true
	Enabled() bool

	// IsGroupActive returns true if this Agent should run checks belonging to groupID.
	// A group this Agent holds no assignment for is active: an unassigned group, or one
	// whose assignment was just removed, must keep reporting rather than go silent.
	IsGroupActive(groupID string) bool

	// GetGroupState returns the state this Agent holds for groupID
	GetGroupState(groupID string) State

	// GetGroupStates returns a copy of every group state this Agent holds
	GetGroupStates() map[string]State
}
