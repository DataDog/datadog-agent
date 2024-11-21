// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent handles states for HA Agent feature.
package haagent

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	// Enabled returns true if ha_agent.enabled is set to true
	Enabled() bool

	// GetGroup returns the value of ha_agent.group
	GetGroup() string

	// IsLeader returns true if the current Agent is leader
	IsLeader() bool

	// SetLeader takes the leader agent hostname as input, if it matches the current agent hostname,
	// the isLeader state is set to true, otherwise false.
	SetLeader(leaderAgentHostname string)
}
