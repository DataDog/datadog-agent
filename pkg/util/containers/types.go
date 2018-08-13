// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package containers

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// Known container runtimes
const (
	RuntimeNameDocker     string = "docker"
	RuntimeNameContainerd string = "containerd"
	RuntimeNameCRIO       string = "cri-o"
)

// Expose container states
const (
	ContainerCreatedState    string = "created"
	ContainerRunningState    string = "running"
	ContainerRestartingState string = "restarting"
	ContainerPausedState     string = "paused"
	ContainerExitedState     string = "exited"
	ContainerDeadState       string = "dead"
)

var (
	// NullContainer is an empty container object that has
	// default values for all fields including sub-fields.
	// If new sub-structs are added to Container this must
	// be updated.
	NullContainer = &Container{
		CPU:     &metrics.CgroupTimesStat{},
		Memory:  &metrics.CgroupMemStat{},
		IO:      &metrics.CgroupIOStat{},
		Network: metrics.ContainerNetStats{},
	}
)

// Container represents a single container on a machine
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
	SoftMemLimit   uint64
	MemLimit       uint64
	CPUNrThrottled uint64
	CPU            *metrics.CgroupTimesStat
	Memory         *metrics.CgroupMemStat
	IO             *metrics.CgroupIOStat
	Network        metrics.ContainerNetStats
	StartedAt      int64

	// For internal use only
	cgroup *metrics.ContainerCgroup
}
