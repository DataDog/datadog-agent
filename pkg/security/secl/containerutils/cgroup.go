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
	CgroupManagerUndefined CGroupManager = iota // unknown
	CGroupManagerDocker                         // docker
	CGroupManagerCRIO                           // cri-o
	CGroupManagerPodman                         // podman
	CGroupManagerCRI                            // containerd
	CGroupManagerSystemd                        // systemd
	CGroupManagerECS                            // ecs
)

// CGroup flags
const (
	SystemdService CGroupFlags = iota + (1 << 8)
	SystemdScope
)

// RuntimeToken holds the cgroup token used by the different runtimes
var RuntimeToken = []struct {
	token string
	flags CGroupManager
}{
	{"docker/", CGroupManagerDocker}, // On Amazon Linux 2 with Docker, 'docker' is the folder name and not a prefix
	{"docker-", CGroupManagerDocker},
	{"cri-containerd-", CGroupManagerCRI},
	{"crio-", CGroupManagerCRIO},
	{"libpod-", CGroupManagerPodman},
	{"ecs/", CGroupManagerECS},

	// fallback to containerd in case of kubepods a
	{"kubepods", CGroupManagerCRI},
}

func getCGroupManager(cgroupID CGroupID) CGroupManager {
	for _, rt := range RuntimeToken {
		if strings.Contains(string(cgroupID), rt.token) {
			return rt.flags
		}
	}
	return CgroupManagerUndefined
}
