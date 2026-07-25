// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentlifecycle defines the experimental prepared/active Agent lifecycle component.
package agentlifecycle

import "context"

// team: agent-runtimes

const (
	// StatePrepared means construction is complete and the process is waiting for node ownership.
	StatePrepared = "prepared"
	// StateActivating means node ownership was acquired and Agent components are starting.
	StateActivating = "activating"
	// StateActive means the Agent process completed startup.
	StateActive = "active"
	// StateStopped means the Agent stopped and released node ownership.
	StateStopped = "stopped"
)

// Params identifies the Agent process using the lifecycle gate.
type Params struct {
	ComponentName string
}

// Component gates Agent startup until older sibling Pods have left the node.
type Component interface {
	// Wait reports Prepared and blocks until no older Pod owned by the same DaemonSet remains on the node.
	Wait(context.Context) error
	// MarkActive reports that Agent startup completed.
	MarkActive() error
	// Close releases node-local ownership after Agent components have stopped.
	Close() error
}
