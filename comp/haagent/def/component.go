// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagent handles states for HA Agent feature.
package haagent

// team: ndm-core

// Component is the component type.
type Component interface {
	// Enabled returns true if ha_agent.enabled is set to true
	Enabled() bool

	// GetConfigID returns the value of config_id
	GetConfigID() string

	// GetState returns current HA agent state
	GetState() State

	// SetLeader takes the leader agent hostname as input, if it matches the current agent hostname,
	// the state is set to active, otherwise standby.
	SetLeader(leaderAgentHostname string)

	// IsActive returns true if the agent should run checks
	IsActive() bool
}
