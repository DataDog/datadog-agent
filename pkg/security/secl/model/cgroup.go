// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"strings"
)

// CGroup managers
const (
	CGroupManagerDocker uint64 = iota + 1
	CGroupManagerCRIO
	CGroupManagerPodman
	CGroupManagerCRI
	CGroupManagerSystemd
)

const (
	// ContainerRuntimeDocker is used to specify that a container is managed by Docker
	ContainerRuntimeDocker = "docker"
	// ContainerRuntimeCRI is used to specify that a container is managed by containerd
	ContainerRuntimeCRI = "containerd"
	// ContainerRuntimeCRIO is used to specify that a container is managed by CRI-O
	ContainerRuntimeCRIO = "cri-o"
	// ContainerRuntimePodman is used to specify that a container is managed by Podman
	ContainerRuntimePodman = "podman"
)

// RuntimePrefixes holds the cgroup prefixed used by the different runtimes
var RuntimePrefixes = map[string]uint64{
	"docker-":         CGroupManagerDocker,
	"cri-containerd-": CGroupManagerCRI,
	"crio-":           CGroupManagerCRIO,
	"libpod-":         CGroupManagerPodman,
}

// GetContainerFromCgroup extracts the container ID from a cgroup name
func GetContainerFromCgroup(cgroup string) (string, uint64) {
	for runtimePrefix, runtimeFlag := range RuntimePrefixes {
		if strings.HasPrefix(cgroup, runtimePrefix) {
			return cgroup[len(runtimePrefix):], runtimeFlag
		}
	}
	return cgroup, 0
}

// GetCgroupFromContainer infers the container runtime from a cgroup name
func GetCgroupFromContainer(id ContainerID, flags uint64) CGroupID {
	for runtimePrefix, runtimeFlag := range RuntimePrefixes {
		if flags&0b111 == runtimeFlag {
			return CGroupID(runtimePrefix + string(id))
		}
	}
	return CGroupID(id)
}
