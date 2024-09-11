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

// IsContainer returns whether a cgroup maps to a container
func (f CGroupFlags) IsContainer() bool {
	return (f&0b111 != 0) && ((f & 0b111) != CGroupFlags(CGroupManagerSystemd))
}
