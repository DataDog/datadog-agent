// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

// GetSelfContainerID returns the container ID of the current process.
//
// It works by comparing the inode of the process's cgroup namespace root
// against all cgroup directories in the host cgroup filesystem, which avoids
// relying on /proc/self/cgroup paths that are relative to the cgroup namespace
// and therefore unusable when running in a private cgroup namespace.
func GetSelfContainerID() (string, error) {
	selfReader, err := cgroups.NewSelfReader("/proc", true,
		cgroups.WithCgroupV1BaseController("memory"),
	)
	if err != nil {
		return "", err
	}

	selfCgroup := selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if selfCgroup == nil {
		return "", errors.New("unable to find self cgroup")
	}

	hostReader, err := cgroups.NewReader(
		cgroups.WithReaderFilter(cgroups.ContainerFilter),
	)
	if err != nil {
		return "", err
	}

	if err := hostReader.RefreshCgroups(0); err != nil {
		return "", err
	}

	cg := hostReader.GetCgroupByInode(selfCgroup.Inode())
	if cg == nil {
		return "", errors.New("container not found by inode in host cgroup filesystem")
	}

	return cg.Identifier(), nil
}
