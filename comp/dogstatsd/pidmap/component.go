// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pidmap implements a component for tracking pid and containerID relations
package pidmap

// team: agent-metrics-logs

// Component is the component type.
type Component interface {

	// SetPidMap sets the map with the pid - containerID relations
	SetPidMap(m map[int32]string)

	// ContainerIDForPID returns the matching container id for a pid, or an error if not found.
	ContainerIDForPID(pid int32) (string, error)
}
