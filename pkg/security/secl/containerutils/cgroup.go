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

// CGroup flags
const (
	SystemdService CGroupFlags = (0 << 8)
	SystemdScope   CGroupFlags = (1 << 8)
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
var RuntimePrefixes = []struct {
	prefix string
	flags  CGroupManager
}{
	{"docker/", CGroupManagerDocker}, // On Amazon Linux 2 with Docker, 'docker' is the folder name and not a prefix
	{"docker-", CGroupManagerDocker},
	{"cri-containerd-", CGroupManagerCRI},
	{"crio-", CGroupManagerCRIO},
	{"libpod-", CGroupManagerPodman},
}

// getContainerFromCgroup extracts the container ID from a cgroup name
func getContainerFromCgroup(cgroup CGroupID) (ContainerID, CGroupFlags) {
	cgroupID := strings.TrimLeft(string(cgroup), "/")
	for _, runtimePrefix := range RuntimePrefixes {
		if strings.HasPrefix(cgroupID, runtimePrefix.prefix) {
			return ContainerID(cgroupID[len(runtimePrefix.prefix):]), CGroupFlags(runtimePrefix.flags)
		}
	}
	return "", 0
}
