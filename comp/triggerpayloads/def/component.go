// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package triggerpayloads sends agent payloads on demand, without waiting for their next scheduled round
package triggerpayloads

import "context"

// team: fleet-remediation

const (
	// PayloadInventoryAgent is the inventory agent metadata payload
	PayloadInventoryAgent = "inventory-agent"
	// PayloadInventoryHost is the inventory host metadata payload
	PayloadInventoryHost = "inventory-host"
	// PayloadInventoryChecks is the inventory checks metadata payload
	PayloadInventoryChecks = "inventory-checks"
	// PayloadAgentHealth is the health platform report
	PayloadAgentHealth = "agent-health"
)

// Component is the component type.
type Component interface {
	// Trigger sends the given payloads immediately and in parallel. An empty list sends all the payloads.
	// A failing payload doesn't prevent the others from being sent.
	Trigger(ctx context.Context, payloads []string) error
}
