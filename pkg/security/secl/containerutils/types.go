// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils groups multiple container utils function that can be used by the secl package
package containerutils

// ContainerID represents a container ID
type ContainerID string

// CGroupID represents a cgroup ID
type CGroupID string

// CGroupFlags represents the flags of a cgroup
type CGroupFlags uint64

// CGroupManagerMask holds the bitmask for the cgroup manager
const CGroupManagerMask CGroupFlags = 0xff

// IsContainer returns whether a cgroup maps to a container
func (f CGroupFlags) IsContainer() bool {
	cgroupManager := f.GetCGroupManager()
	return cgroupManager != 0 && cgroupManager != CGroupManagerSystemd
}

// IsSystemd returns whether a cgroup maps to a systemd cgroup
func (f CGroupFlags) IsSystemd() bool {
	return f.GetCGroupManager() == CGroupManagerSystemd
}

// GetCGroupManager returns the cgroup manager from the flags
func (f CGroupFlags) GetCGroupManager() CGroupManager {
	return CGroupManager(f & CGroupManagerMask)
}
