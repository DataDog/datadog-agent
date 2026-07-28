// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentlifecycle defines the experimental Agent startup gate.
package agentlifecycle

import "context"

// team: agent-runtimes

// Params identifies the Agent process using the lifecycle gate.
type Params struct {
	ComponentName string
}

// Component gates Agent construction until the older same-name container has stopped.
type Component interface {
	// Wait blocks until no older same-name container owned by the same DaemonSet remains on the node.
	Wait(context.Context) error
	// Close releases gate resources.
	Close() error
}
