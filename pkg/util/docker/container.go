// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

// Expose container states
const (
	ContainerCreatedState    string = "created"
	ContainerRunningState    string = "running"
	ContainerRestartingState string = "restarting"
	ContainerPausedState     string = "paused"
	ContainerExitedState     string = "exited"
	ContainerDeadState       string = "dead"
)

// Container represents a single Docker container on a machine
// and includes Cgroup-level statistics about the container.
type Container struct {
	Type     string
	ID       string
	EntityID string
	Name     string
	Image    string
	ImageID  string
	Created  int64
	State    string
	Health   string
	Pids     []int32
	Excluded bool

	CPULimit       float64
	MemLimit       uint64
	CPUNrThrottled uint64
	CPU            *CgroupTimesStat
	Memory         *CgroupMemStat
	IO             *CgroupIOStat
	Network        ContainerNetStats
	StartedAt      int64

	// For internal use only
	cgroup *ContainerCgroup
}
