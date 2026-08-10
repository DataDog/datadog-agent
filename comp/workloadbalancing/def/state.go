// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancing

// State type for the state an Agent holds for a workload balancing group
type State string

const (
	// Active means this Agent runs the group's checks
	Active State = "active"
	// Standby means another Agent runs the group's checks
	Standby State = "standby"
	// Unmanaged means this Agent holds no assignment for the group, and runs its checks
	Unmanaged State = "unmanaged"
)
