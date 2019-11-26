// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package containers

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
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

// Container network modes
const (
	DefaultNetworkMode string = "default" // bridge
	HostNetworkMode           = "host"
	BridgeNetworkMode         = "bridge"
	NoneNetworkMode           = "none"
	AwsvpcNetworkMode         = "awsvpc"
	UnknownNetworkMode        = "unknown"
)

// UTSMode is container UTS modes
type UTSMode string

// UTSMode is container UTS modes
const (
	DefaultUTSMode UTSMode = ""
	HostUTSMode            = "host"
	UnknownUTSMode         = "unknown"
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
	KernMemUsage   uint64
	MemLimit       uint64
	MemFailCnt     uint64
	CPUNrThrottled uint64
	CPU            *metrics.CgroupTimesStat
	Memory         *metrics.CgroupMemStat
	IO             *metrics.CgroupIOStat
	Network        metrics.ContainerNetStats
	AddressList    []NetworkAddress
	StartedAt      int64
	ThreadCount    uint64
	ThreadLimit    uint64

	// For internal use only
	cgroup *metrics.ContainerCgroup
}

// NetworkAddress represents a tuple IP/Port/Protocol
type NetworkAddress struct {
	IP       net.IP
	Port     int
	Protocol string
}
