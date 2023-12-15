// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	// We wrap pkg/security/utils here only for compat reason to be able to
	// still compile pkg/compliance on !linux.
	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
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
