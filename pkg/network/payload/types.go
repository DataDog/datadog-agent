// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payload separates network types used as JSON payloads into a module
package payload

// Via has info about the routing decision for a flow
type Via struct {
	Subnet    Subnet        `json:"subnet,omitempty"`
	Interface Interface     `json:"interface,omitempty"`
	SG        SecurityGroup `json:"security_group,omitempty"`
}

// Interface has information about a network interface
type Interface struct {
	HardwareAddr string `json:"hardware_addr,omitempty"`
}

// Subnet stores info about a subnet
type Subnet struct {
	Alias string `json:"alias,omitempty"`
}

type SecurityGroup struct {
	ID string `json:"id,omitempty"`
}
