// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagent

// State type for HA Agent State
type State string

const (
	// Active HA Agent state
	Active State = "active"
	// Standby HA Agent state
	Standby State = "standby"
	// Unknown HA Agent state
	Unknown State = "unknown"
)
