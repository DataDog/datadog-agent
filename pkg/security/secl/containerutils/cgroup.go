// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate stringer -type=CGroupManager -linecomment -output cgroup_strings.go

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"strings"
)

// CGroupManager holds the manager of the cgroup lifecycle
type CGroupManager uint64

// CGroup managers
const (
	CGroupManagerDocker  CGroupManager = iota + 1 // docker
	CGroupManagerCRIO                             // cri-o
	CGroupManagerPodman                           // podman
	CGroupManagerCRI                              // containerd
	CGroupManagerSystemd                          // systemd
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
var RuntimePrefixes = map[string]CGroupManager{
	"docker/":         CGroupManagerDocker, // On Amazon Linux 2 with Docker, 'docker' is the folder name and not a prefix
	"docker-":         CGroupManagerDocker,
	"cri-containerd-": CGroupManagerCRI,
	"crio-":           CGroupManagerCRIO,
	"libpod-":         CGroupManagerPodman,
}

// GetCGroupManager extracts the cgroup manager from a cgroup name
func GetCGroupManager(cgroup string) (string, CGroupFlags) {
	for runtimePrefix, runtimeFlag := range RuntimePrefixes {
		if strings.HasPrefix(cgroup, runtimePrefix) {
			return cgroup[:len(runtimePrefix)], CGroupFlags(runtimeFlag)
		}
	}
	return cgroup, 0
}

// GetContainerFromCgroup extracts the container ID from a cgroup name
func GetContainerFromCgroup(cgroup string) (string, CGroupFlags) {
	for runtimePrefix, runtimeFlag := range RuntimePrefixes {
		if strings.HasPrefix(cgroup, runtimePrefix) {
			return cgroup[len(runtimePrefix):], CGroupFlags(runtimeFlag)
		}
	}
	return "", 0
}

// GetCgroupFromContainer infers the container runtime from a cgroup name
func GetCgroupFromContainer(id ContainerID, flags CGroupFlags) CGroupID {
	for runtimePrefix, runtimeFlag := range RuntimePrefixes {
		if uint64(flags)&0b111 == uint64(runtimeFlag) {
			return CGroupID(runtimePrefix + string(id))
		}
	}
	return CGroupID("")
}
