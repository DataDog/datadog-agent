// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"errors"
	"maps"
)

var (
	// ErrNotImplemented is the "not implemented" error given by `gopsutil` when an
	// OS doesn't support an API. Unfortunately it's in an internal package so
	// we can't import it so we'll copy it here.
	ErrNotImplemented = errors.New("not implemented yet")

	// ErrDockerNotAvailable is returned if Docker is not running on the current machine.
	// We'll use this when configuring the DockerUtil so we don't error on non-docker machines.
	ErrDockerNotAvailable = errors.New("docker not available")

	// ErrDockerNotCompiled is returned if docker support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrDockerNotCompiled = errors.New("docker support not compiled in")
)

// Container network modes
const (
	DefaultNetworkMode string = "default" // bridge
	HostNetworkMode    string = "host"
	BridgeNetworkMode  string = "bridge"
	NoneNetworkMode    string = "none"
	AwsvpcNetworkMode  string = "awsvpc"
	UnknownNetworkMode string = "unknown"
)

// ContainerHosts returns a map of hostnames to IP addresses for a container.
// It includes the container's network IP addresses, the rancher IP if
// available, and container's hostname if no IP is available.
func ContainerHosts(networkIPs, labels map[string]string, hostname string) map[string]string {
	hosts := make(map[string]string)

	maps.Copy(hosts, networkIPs)

	if rancherIP, ok := FindRancherIPInLabels(labels); ok {
		hosts["rancher"] = rancherIP
	}

	// Some CNI solutions (including ECS awsvpc) do not assign an
	// IP through docker, but set a valid reachable hostname. Use
	// it if no IP is discovered.
	if len(hosts) == 0 && len(hostname) > 0 {
		hosts["hostname"] = hostname
	}
	return hosts
}
