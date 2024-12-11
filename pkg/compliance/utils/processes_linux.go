// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	// We wrap pkg/security/utils here only for compat reason to be able to
	// still compile pkg/compliance on !linux.
	"fmt"

	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ContainerID wraps a string representing a container identifier.
type ContainerID string

// GetProcessContainerID returns the container ID associated with the given
// process ID. Returns an empty string if no container found.
func GetProcessContainerID(pid int32) (ContainerID, bool) {
	containerID, err := secutils.GetProcContainerID(uint32(pid), uint32(pid))
	if containerID == "" || err != nil {
		return "", false
	}
	return ContainerID(containerID), true
}

// GetProcessRootPath returns the process root path of the given PID.
func GetProcessRootPath(pid int32) (string, bool) {
	return secutils.ProcRootPath(uint32(pid)), true
}

// GetContainerOverlayPath tries to extract the directory mounted as root
// mountpoint of the given process. To do so it parses the mountinfo table of
// the process and tries to match it with the mount entry of the root
// namespace (mountinfo pid 1).
func GetContainerOverlayPath(pid int32) (string, error) {
	nsMounts, err := kernel.ParseMountInfoFile(pid)
	if err != nil {
		return "", err
	}
	var overlayOptions string
	for _, mount := range nsMounts {
		if mount.Mountpoint == "/" && mount.FSType == "overlay" {
			overlayOptions = mount.VFSOptions
			break
		}
	}
	if overlayOptions != "" {
		rootMounts, err := kernel.ParseMountInfoFile(1)
		if err != nil {
			return "", err
		}
		for _, mount := range rootMounts {
			if mount.FSType == "overlay" && mount.Root == "/" && mount.VFSOptions == overlayOptions {
				return mount.Mountpoint, nil
			}
		}
	}
	return "", fmt.Errorf("could not find overlay mountpoint")
}
