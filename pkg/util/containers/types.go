// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package containers

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"net"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/util/containers/metrics"
)

// Known container runtimes
const (
	RuntimeNameDocker     string = "docker"
	RuntimeNameContainerd string = "containerd"
	RuntimeNameCRIO       string = "cri-o"
)

// Supported container states
const (
	ContainerUnknownState    string = "unknown"
	ContainerCreatedState           = "created"
	ContainerRunningState           = "running"
	ContainerRestartingState        = "restarting"
	ContainerPausedState            = "paused"
	ContainerExitedState            = "exited"
	ContainerDeadState              = "dead"
)

// Supported container health
const (
	ContainerUnknownHealth  string = "unknown"
	ContainerStartingHealth        = "starting"
	ContainerHealthy               = "healthy"
	ContainerUnhealthy             = "unhealthy"
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
	MemFailCnt     uint64
	CPUNrThrottled uint64
	CPU            *metrics.CgroupTimesStat
	Memory         *metrics.CgroupMemStat
	IO             *metrics.CgroupIOStat
	Network        metrics.ContainerNetStats
	AddressList    []NetworkAddress
	StartedAt      int64

	Mounts []types.MountPoint

	// For internal use only
	cgroup *metrics.ContainerCgroup
}

// NetworkAddress represents a tuple IP/Port/Protocol
type NetworkAddress struct {
	IP       net.IP
	Port     int
	Protocol string
}

// SwarmService represents a Swarm Service definition
// sts
type SwarmService struct {
	ID             string
	Name           string
	ContainerImage string
	Labels         map[string]string  `json:",omitempty"`
	Version        swarm.Version      `json:",omitempty"`
	CreatedAt      time.Time          `json:",omitempty"`
	UpdatedAt      time.Time          `json:",omitempty"`
	Spec           swarm.ServiceSpec  `json:",omitempty"`
	PreviousSpec   *swarm.ServiceSpec `json:",omitempty"`
	Endpoint       swarm.Endpoint     `json:",omitempty"`
	UpdateStatus   swarm.UpdateStatus `json:",omitempty"`
	TaskContainers []*SwarmTask
	DesiredTasks   uint64
	RunningTasks   uint64
}

// SwarmTask represents a Swarm TaskContainer definition
// sts
type SwarmTask struct {
	ID              string
	Name            string
	ContainerImage  string
	ContainerSpec   swarm.ContainerSpec   `json:",omitempty"`
	ContainerStatus swarm.ContainerStatus `json:",omitempty"`
	DesiredState    swarm.TaskState       `json:",omitempty"`
}
