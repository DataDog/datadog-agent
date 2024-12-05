// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagent

// Role type for HA Agent Role
type Role string

const (
	// Primary HA Agent role
	Primary Role = "primary"
	// Standby HA Agent role
	Standby Role = "standby"
	// Unknown HA Agent role
	Unknown Role = "unknown"
)
