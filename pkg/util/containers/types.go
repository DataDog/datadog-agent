// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

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
// and includes system-level statistics about the container.
type Container struct {
	Type        string
	ID          string
	EntityID    string
	Name        string
	Image       string
	ImageID     string
	Created     int64
	State       string
	Health      string
	Pids        []int32
	Excluded    bool
	AddressList []NetworkAddress
	StartedAt   int64

	metrics.ContainerLimits
	metrics.ContainerMetrics
	Network metrics.ContainerNetStats
}

// NetworkAddress represents a tuple IP/Port/Protocol
type NetworkAddress struct {
	IP       net.IP
	Port     int
	Protocol string
}

// NetworkDestination holds one network destination subnet and it's linked interface name
type NetworkDestination struct {
	Interface string
	Subnet    uint64
	Mask      uint64
}

// ContainerImplementation is a generic interface that defines a common interface across
// different container implementation (Linux cgroup, windows containers, etc.)
type ContainerImplementation interface {
	// Asks provider to fetch data from system APIs in bulk
	// It's be required to call it before any other function
	Prefetch() error

	ContainerExists(containerID string) bool
	GetContainerStartTime(containerID string) (int64, error)
	DetectNetworkDestinations(pid int) ([]NetworkDestination, error)
	GetAgentCID() (string, error)
	ContainerIDForPID(pid int) (string, error)

	metrics.ContainerMetricsProvider
}
